package function

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v72/github"
)

type ReleasePromoter struct {
	webhookSecret string
	privateKey    string
	appID         int64
	client        *github.Client
	debounceTimer map[string]*time.Timer
	debounceMutex sync.RWMutex
	cleanupTicker *time.Ticker
	done          chan bool
}

type PendingRelease struct {
	Owner     string
	Repo      string
	ReleaseID int64
}

var promoter *ReleasePromoter

func init() {
	webhookSecretBytes, err := os.ReadFile("/var/openfaas/secrets/release-promoter-webhook")
	if err != nil {
		log.Fatalf("Failed to read webhook secret: %v", err)
	}

	privateKeyBytes, err := os.ReadFile("/var/openfaas/secrets/release-promoter-private-key")
	if err != nil {
		log.Fatalf("Failed to read private key: %v", err)
	}

	appIDStr := os.Getenv("app_id")
	if appIDStr == "" {
		log.Fatalf("app_id environment variable not set")
	}
	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		log.Fatalf("Invalid app_id: %v", err)
	}

	promoter = &ReleasePromoter{
		webhookSecret: strings.TrimSpace(string(webhookSecretBytes)),
		privateKey:    strings.TrimSpace(string(privateKeyBytes)),
		appID:         appID,
		debounceTimer: make(map[string]*time.Timer),
	}

	if err := promoter.setupGitHubClient(); err != nil {
		log.Fatalf("Failed to setup GitHub client: %v", err)
	}

	log.Printf("[init] release promoter initialized with GitHub App ID: %d", appID)
}

func (rp *ReleasePromoter) setupGitHubClient() error {
	atr, err := ghinstallation.NewAppsTransport(
		http.DefaultTransport,
		rp.appID,
		[]byte(rp.privateKey),
	)
	if err != nil {
		return fmt.Errorf("failed to create apps transport: %w", err)
	}

	rp.client = github.NewClient(&http.Client{Transport: atr})
	return nil
}

func (rp *ReleasePromoter) createInstallationClient(installationID int64) *github.Client {
	itr, err := ghinstallation.New(
		http.DefaultTransport,
		rp.appID,
		installationID,
		[]byte(rp.privateKey),
	)
	if err != nil {
		log.Printf("Failed to create installation transport: %v", err)
		return nil
	}
	return github.NewClient(&http.Client{Transport: itr})
}

func (rp *ReleasePromoter) validateWebhook(r *http.Request, body []byte) error {
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		return fmt.Errorf("missing X-Hub-Signature-256 header")
	}

	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("invalid signature format")
	}

	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(rp.webhookSecret))
	mac.Write(body)
	expectedMAC := mac.Sum(nil)

	if !hmac.Equal(sig, expectedMAC) {
		return fmt.Errorf("webhook signature validation failed")
	}

	return nil
}

func (rp *ReleasePromoter) handleReleaseEdit(event *github.ReleaseEvent) error {
	if event.GetAction() != "edited" {
		return nil
	}

	release := event.GetRelease()
	if release == nil {
		return fmt.Errorf("release is nil")
	}

	if !release.GetPrerelease() {
		log.Printf("Release %s is already not a pre-release, skipping", release.GetName())
		return nil
	}

	assets := release.Assets
	if assets == nil || len(assets) == 0 {
		log.Printf("No assets found for release %s, skipping", release.GetName())
		return nil
	}

	// Extract installation ID from webhook
	installationID := event.GetInstallation().GetID()
	if installationID == 0 {
		return fmt.Errorf("no installation ID in webhook")
	}

	owner := event.GetRepo().GetOwner().GetLogin()
	repo := event.GetRepo().GetName()
	releaseID := release.GetID()

	log.Printf("[release] processing: %s/%s (pre-release: %v, assets: %d, installation_id: %d)", owner, repo, release.GetPrerelease(), len(assets), installationID)

	rp.scheduleDebounce(owner, repo, releaseID, installationID)
	return nil
}

func (rp *ReleasePromoter) scheduleDebounce(owner, repo string, releaseID, installationID int64) {
	key := fmt.Sprintf("%s-%s-%d", owner, repo, releaseID)

	rp.debounceMutex.Lock()
	defer rp.debounceMutex.Unlock()

	if timer, exists := rp.debounceTimer[key]; exists {
		timer.Stop()
		log.Printf("[debounce] resetting timer for %s/%s release %d", owner, repo, releaseID)
	}

	rp.debounceTimer[key] = time.AfterFunc(10*time.Second, func() {
		rp.clearDebounceTimer(key)

		rp.promoteRelease(owner, repo, releaseID, installationID)
	})
	log.Printf("[debounce] scheduled 10s debounce for %s/%s release %d", owner, repo, releaseID)
}

func (rp *ReleasePromoter) clearDebounceTimer(key string) {
	rp.debounceMutex.Lock()
	defer rp.debounceMutex.Unlock()
	delete(rp.debounceTimer, key)
}

func (rp *ReleasePromoter) promoteRelease(owner, repo string, releaseID, installationID int64) {
	start := time.Now()
	log.Printf("[github] calling API to promote release %s/%s/%d (installation_id: %d)", owner, repo, releaseID, installationID)

	// Create installation-specific client
	client := rp.createInstallationClient(installationID)
	if client == nil {
		log.Printf("Failed to create installation client for installation ID: %d", installationID)
		return
	}

	release := &github.RepositoryRelease{
		Prerelease: github.Ptr(false),
	}

	if _, res, err := client.Repositories.EditRelease(context.Background(), owner, repo, releaseID, release); err != nil {
		log.Printf("Failed to promote release: %v", err)

		if res.Body != nil {
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)

			log.Printf("Response body: %s", string(body))
		}
		return
	}

	log.Printf("[success] promoted release %s/%s/%d in %v", owner, repo, releaseID, time.Since(start))
}

func (rp *ReleasePromoter) handlePing(body []byte) error {
	var pingEvent struct {
		Zen    string `json:"zen"`
		HookID int64  `json:"hook_id"`
		Hook   struct {
			Type   string   `json:"type"`
			ID     int64    `json:"id"`
			Name   string   `json:"name"`
			Active bool     `json:"active"`
			Events []string `json:"events"`
			AppID  int64    `json:"app_id"`
		} `json:"hook"`
	}

	if err := json.Unmarshal(body, &pingEvent); err != nil {
		return fmt.Errorf("failed to parse ping event: %w", err)
	}

	log.Printf("[ping] received ping - hook_id: %d, app_id: %d, events: %v, zen: %s",
		pingEvent.HookID, pingEvent.Hook.AppID, pingEvent.Hook.Events, pingEvent.Zen)

	return nil
}

func Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := promoter.validateWebhook(r, body); err != nil {
		log.Printf("Webhook validation failed: %v", err)
		http.Error(w, "Invalid webhook signature", http.StatusUnauthorized)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	log.Printf("[webhook] processing event: %s from %s", eventType, r.RemoteAddr)

	switch eventType {
	case "ping":
		if err := promoter.handlePing(body); err != nil {
			log.Printf("Failed to handle ping: %v", err)
			http.Error(w, "Failed to process ping", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ping event processed"))

	case "release":
		var event github.ReleaseEvent
		if err := json.Unmarshal(body, &event); err != nil {
			log.Printf("Failed to parse release event: %v", err)
			http.Error(w, "Failed to parse event", http.StatusBadRequest)
			return
		}

		if err := promoter.handleReleaseEdit(&event); err != nil {
			log.Printf("Failed to handle release edit: %v", err)
			http.Error(w, "Failed to process event", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Release edit event processed"))

	default:
		log.Printf("[webhook] unhandled event type: %s", eventType)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Event type not handled"))
		return
	}
}
