## release-promoter

**Promote GitHub releases to latest when their uploads are published.**

![Logo](/images/logo.png)

You cut a release which will have binaries added only after the CI job is fully completed.

![](/images/before-pre.png)

Then after all the binaries have been built, and uploaded, after a 10 second debounce period, the release is promoted to latest.

![](/images/after-latest.png)

The logs show the function receiving the webhook, and promoting the release.

![](/images/logs-dashboard.png)

The example uses [alexellis/upload-assets-testing](https://github.com/alexellis/upload-assets-testing) but you can use this with any repository.

### Getting the Release Promoter

1) Install OpenFaaS into a VM or a Kubernetes cluster, then build and deploy your own version of this function with your own GitHub App.
2) Install our managed version using this link and our [hosted GitHub App](https://github.com/apps/release-promoter-function) running on OpenFaaS Edge - completely free with no ongoing costs or maintenance.

### About this code

This is an OpenFaaS Function written with the [golang-middleware template](https://docs.openfaas.com/go).

It can be deployed via `faas-cli up --publish --gateway https://gateway.example.com`.

You'll also need to create a GitHub App:

* Contents read/write - in order to Edit releases
* Webhooks - Tick "Release events"

For the webhook URL, use your OpenFaaS gateway URL with the function name, e.g.

```
https://gateway.example.com/function/release-promoter
```

Then install the GitHub App on your repositories or your whole organization.

The private key should be downloaded and saved to `.secrets/release-promoter-private-key`
The webhook secret should be saved to `.secrets/release-promoter-webhook-secret`

Then upload them to your gateway:

```bash
export OPENFAAS_URL=https://gateway.example.com
faas-cli secret create release-promoter-private-key --from-file=.secrets/release-promoter-private-key
faas-cli secret create release-promoter-webhook-secret --from-file=.secrets/release-promoter-webhook-secret
```

Finally, publish an image and deploy the function to your OpenFaaS gateway:

```bash
SERVER=docker.io \
OWNER=owner \
REPO=release-promoter \
faas-cli up --publish --tag=digest
```

## How can I get OpenFaaS?

This function works on:

| Product | Platform| Usage | Costs |
|----------|-----|-------|-------|
| faasd CE | Self-hosted Linux VM | Free for personal use only - limited features and number of functions | Free |
| OpenFaaS Edge (for sponsors) | Self-hosted Linux VM | Includes extra features, up to 25 functions and scale to zero | [25 USD / mo](https://github.com/sponsors/alexellis) |
| OpenFaaS Edge (commercial use) | Self-hosted Linux VM | Includes extra features, up to 25 functions and scale to zero | [Custom bundle](https://docs.google.com/forms/d/e/1FAIpQLSe2O9tnlTjc7yqzXLMvqvF2HVqwNW7ePNOxLchacKRf9LZL7Q/viewform) |
| OpenFaaS CE for Kubernetes | Self-hosted on Kubernetes/K3s | Free for personal use only with limited number of functions | Free |
| OpenFaaS Standard for Kubernetes |Self-hosted on Kubernetes/K3s | For commercial use up to 500 functions included and many extra features | [See website](https://www.openfaas.com/pricing/) |

