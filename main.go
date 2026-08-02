// Command gcloud-ctx is a fast context switcher for gcloud named
// configurations, kubectx-style, that also keeps Application Default
// Credentials and service account impersonation in sync across a switch.
package main

import (
	"os"

	"github.com/sakajunquality/gcloud-ctx/internal/cli"
)

// version, commit, and date are set via -ldflags at build time (see
// GoReleaser config); "dev"/"none"/"unknown" are the defaults for
// `go build`/`go run`.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(cli.Execute(version, commit, date))
}
