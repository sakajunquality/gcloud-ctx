// Package cli implements gcloud-ctx's command-line interface: the
// kubectx-style root command grammar plus the create, show, impersonate, and
// adc save subcommands. It is thin orchestration over internal/gcloud,
// internal/adc, and internal/store.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/sakajunquality/gcloud-ctx/internal/ui"
)

// errAlreadyReported signals that a command already printed its own
// "error: ..." lines (e.g. one per failed name in a multi-name delete) and
// Execute should just exit non-zero without printing anything further.
var errAlreadyReported = errors.New("gcloud-ctx: already reported")

// Execute builds the root command, runs it against os.Args, and returns the
// process exit code: 0 on success, 130 if the interactive picker was
// cancelled, 1 on any other error (printed to stderr as "error: ...").
func Execute(version, commit, date string) int {
	root := NewRootCmd(fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date))
	err := root.Execute()
	switch {
	case err == nil:
		return 0
	case errors.Is(err, ui.ErrCancelled):
		return 130
	case errors.Is(err, errAlreadyReported):
		return 1
	default:
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
}
