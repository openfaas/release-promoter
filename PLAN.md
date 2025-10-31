This is a webhook receive using Google's SDK for Go/GitHub v72 (may upgrade this if available)

Use Go 1.25

Goals:

* Validate webhooks against a symmetrical webook secret
* Receive events about releases
* Impersonate the GitHub App installation using a private key and switch the release from pre-release to latest and non-prerelease

Criteria:

* Must have received an edit event where an asset was added.
* Multiple assets can be added, so we have to debounce by i.e. 10s from the last edit with a changed asset before commiting the change

Caveats:

* Some releases may never get binaries due to not having them or to failing CI - in that circumstance we never update the release.
* Code must be super minimal, multiple named files are OK - multiple packages are overkill.
* It will be converted to openfaas' template golang-middleware - so read secrets from /var/openfaas/secrets/NAME

Technical:

* To add a go mod - `cd ./release-promoter` && `go get` etc
* To build the function `faas-cli build` - within the root of the project

