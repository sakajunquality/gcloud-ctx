package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sakajunquality/gcloud-ctx/internal/gcloud"
)

func newCreateCmd() *cobra.Command {
	var (
		project     string
		account     string
		impersonate string
		noActivate  bool
	)

	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "Create a new context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp(cmd)
			if err != nil {
				return err
			}
			props := gcloud.Properties{
				Account:                   account,
				Project:                   project,
				ImpersonateServiceAccount: impersonate,
			}
			return a.create(args[0], props, !noActivate)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "core/project to set on the new context")
	cmd.Flags().StringVar(&account, "account", "", "core/account to set on the new context")
	cmd.Flags().StringVar(&impersonate, "impersonate", "", "auth/impersonate_service_account to set on the new context")
	cmd.Flags().BoolVar(&noActivate, "no-activate", false, "don't switch to the new context")

	return cmd
}

// create implements "gcloud-ctx create": it does not touch ADC — impersonate
// or "adc save" are the explicit ways to set it up afterward.
//
// The ADC hint is printed exactly once: when activate is true, switchTo's
// own "no stored ADC" hint already covers it, so create doesn't repeat
// itself with a second line; only the --no-activate path (where switchTo
// never runs) prints create's own hint.
func (a *app) create(name string, props gcloud.Properties, activate bool) error {
	if err := a.gs.Create(name, props); err != nil {
		return err
	}
	fmt.Fprintf(a.errw, "Created context %q.\n", name)
	if activate {
		return a.switchTo(name)
	}
	fmt.Fprintln(a.errw, "ADC unchanged; run 'gcloud-ctx impersonate' or 'gcloud-ctx adc save' to configure it.")
	return nil
}
