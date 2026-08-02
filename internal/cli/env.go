package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sakajunquality/gcloud-ctx/internal/gcloud"
)

func newEnvCmd() *cobra.Command {
	var unsetFlag bool
	cmd := &cobra.Command{
		Use:   "env [NAME]",
		Short: "Print shell exports that pin a context for one shell only",
		Long: strings.TrimSpace(`
Print POSIX shell export lines that pin a context for the current shell
(and its child processes) only, without touching the machine-global
active_config or live ADC file:

  CLOUDSDK_ACTIVE_CONFIG_NAME       pins what the gcloud CLI uses
  GOOGLE_APPLICATION_CREDENTIALS    pins what ADC consumers (Terraform,
                                    client libraries) use, pointing at the
                                    context's stored ADC snapshot

This is how several shells, agents, or terminal tabs on one machine each
use a different context concurrently. The ADC line is emitted only when
the context has a stored ADC snapshot (bind one with 'gcloud-ctx adc
save'); otherwise the variable is unset so it can't point at a stale
snapshot from a previous eval.

Apply with:  eval "$(gcloud-ctx env NAME)"
Undo with:   eval "$(gcloud-ctx env --unset)"
`),
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeContextNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp(cmd)
			if err != nil {
				return err
			}
			if unsetFlag {
				if len(args) != 0 {
					return fmt.Errorf("--unset takes no NAME argument")
				}
				return a.envUnset()
			}
			var name string
			if len(args) == 1 {
				name = args[0]
			}
			return a.env(name)
		},
	}
	cmd.Flags().BoolVar(&unsetFlag, "unset", false, "print unset lines that undo a previous 'gcloud-ctx env'")
	return cmd
}

// env implements "gcloud-ctx env [NAME]": eval-able exports on stdout, hints
// on stderr. Nothing on disk is read beyond existence checks and nothing is
// written — this is the read-only, per-shell alternative to switching.
func (a *app) env(name string) error {
	if name == "" {
		eff, err := a.gs.EffectiveActiveName()
		if err != nil {
			return err
		}
		if eff == "" {
			return fmt.Errorf("no active configuration set")
		}
		name = eff
	}

	if !gcloud.IsNone(name) {
		if err := gcloud.ValidateName(name); err != nil {
			return err
		}
	}
	exists, err := a.gs.Exists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("configuration %q does not exist", name)
	}

	fmt.Fprintf(a.out, "export %s=%s\n", gcloud.ActiveConfigEnvVar, shellQuote(name))

	hasADC := false
	if !gcloud.IsNone(name) {
		if hasADC, err = a.ss.HasADC(name); err != nil {
			return err
		}
	}
	if hasADC {
		path, err := a.ss.ADCPath(name)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "export GOOGLE_APPLICATION_CREDENTIALS=%s\n", shellQuote(path))
	} else {
		// Explicitly unset rather than omit: eval-ing env for a context
		// without a snapshot must not leave the variable pointing at some
		// other context's credentials from an earlier eval.
		fmt.Fprintln(a.out, "unset GOOGLE_APPLICATION_CREDENTIALS")
		if !gcloud.IsNone(name) {
			fmt.Fprintf(a.errw, "no stored ADC for %q; ADC consumers will fall back to the live ADC file (bind one with 'gcloud-ctx adc save')\n", name)
		}
	}

	fmt.Fprintf(a.out, "# Run this command to configure your shell:\n# eval \"$(gcloud-ctx env %s)\"\n", name)
	return nil
}

// envUnset implements "gcloud-ctx env --unset": undo lines for a prior eval.
func (a *app) envUnset() error {
	fmt.Fprintf(a.out, "unset %s\n", gcloud.ActiveConfigEnvVar)
	fmt.Fprintln(a.out, "unset GOOGLE_APPLICATION_CREDENTIALS")
	fmt.Fprintf(a.out, "# Run this command to configure your shell:\n# eval \"$(gcloud-ctx env --unset)\"\n")
	return nil
}

// shellQuote wraps s in single quotes for POSIX shells, escaping any embedded
// single quote as '\” so the output is always safe to eval.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
