package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sakajunquality/gcloud-ctx/internal/adc"
	"github.com/sakajunquality/gcloud-ctx/internal/gcloud"
	"github.com/sakajunquality/gcloud-ctx/internal/store"
	"github.com/sakajunquality/gcloud-ctx/internal/ui"
)

// liveADCFileName is the well-known name gcloud itself uses for the live ADC
// file, directly under the gcloud config directory.
const liveADCFileName = "application_default_credentials.json"

// app bundles everything a command needs to run: the gcloud named-config
// store, gcloud-ctx's own state store, the command's I/O streams, and the
// color style to use for a.out.
type app struct {
	gs    *gcloud.Store
	ss    *store.Store
	in    io.Reader
	out   io.Writer
	errw  io.Writer
	style ui.Style
}

// newApp resolves the gcloud config directory (honoring CLOUDSDK_CONFIG) and
// wires up an app around cmd's streams.
func newApp(cmd *cobra.Command) (*app, error) {
	gs, err := gcloud.Open()
	if err != nil {
		return nil, err
	}
	out := cmd.OutOrStdout()
	return &app{
		gs:    gs,
		ss:    store.New(gs.Dir),
		in:    cmd.InOrStdin(),
		out:   out,
		errw:  cmd.ErrOrStderr(),
		style: ui.DetectStyle(out),
	}, nil
}

// liveADCPath returns the path to the live ADC file gcloud and client
// libraries read.
func (a *app) liveADCPath() string {
	return filepath.Join(a.gs.Dir, liveADCFileName)
}

// warnEnv prints the env-var footguns called out by DESIGN.md: a shell-local
// active-config override that shadows switches, and an ADC override that
// makes the live ADC file irrelevant to libraries.
func warnEnv(w io.Writer) {
	if v := os.Getenv(gcloud.ActiveConfigEnvVar); v != "" {
		fmt.Fprintf(w, "warning: %s is set to %q; this shell's active context is shadowed\n", gcloud.ActiveConfigEnvVar, v)
	}
	if v := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); v != "" {
		fmt.Fprintf(w, "warning: GOOGLE_APPLICATION_CREDENTIALS is set to %q; libraries ignore the live ADC file\n", v)
	}
}

// dash returns s, or "-" if s is empty — for display of unset properties.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// sanitize strips C0 control characters (0x00-0x1F) from s before display.
// Property values are validated against these on write (see
// internal/gcloud.validatePropertyValue), but a file predating that
// validation, or edited by another tool, could still contain one; stripping
// them here keeps a corrupt value from mangling terminal output or a table's
// layout in list/show.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 {
			return -1
		}
		return r
	}, s)
}

// knownADCTypes are the ADC credential types gcloud-ctx recognizes well
// enough to install as the live ADC file or accept into its own store: the
// types Credential.Type can return that gcloud itself writes.
var knownADCTypes = map[adc.Type]bool{
	adc.TypeAuthorizedUser:                true,
	adc.TypeServiceAccount:                true,
	adc.TypeImpersonatedServiceAccount:    true,
	adc.TypeExternalAccount:               true,
	adc.TypeExternalAccountAuthorizedUser: true,
}

// knownADCType reports whether t is one of knownADCTypes.
func knownADCType(t adc.Type) bool {
	return knownADCTypes[t]
}
