package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sakajunquality/gcloud-ctx/internal/adc"
	"github.com/sakajunquality/gcloud-ctx/internal/gcloud"
)

func newADCCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adc",
		Short: "Manage per-context Application Default Credentials snapshots",
	}
	cmd.AddCommand(newADCSaveCmd())
	return cmd
}

func newADCSaveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "save [NAME]",
		Short:             "Snapshot the live ADC file into the store for NAME (default: current)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeContextNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp(cmd)
			if err != nil {
				return err
			}
			var name string
			if len(args) == 1 {
				name = args[0]
			}
			return a.adcSave(name)
		},
	}
	return cmd
}

// adcSave implements "gcloud-ctx adc save [NAME]": the only way to bind the
// current live ADC to a context — there is no automatic snapshotting.
func (a *app) adcSave(name string) error {
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
	if gcloud.IsNone(name) {
		return fmt.Errorf("cannot bind an ADC to the virtual %s configuration", gcloud.NoneConfig)
	}

	exists, err := a.gs.Exists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("configuration %q does not exist", name)
	}

	data, err := os.ReadFile(a.liveADCPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("live ADC file does not exist; run 'gcloud auth application-default login' first")
		}
		return fmt.Errorf("read live ADC file: %w", err)
	}

	cred, err := adc.Parse(data)
	if err != nil {
		return fmt.Errorf("live ADC file is not valid JSON: %w", err)
	}
	if !knownADCType(cred.Type()) {
		return fmt.Errorf("live ADC file has unrecognized credential type %q", cred.Type())
	}

	if err := a.ss.SaveADC(name, data); err != nil {
		return err
	}
	fmt.Fprintf(a.errw, "ADC saved for context %q.\n", name)
	return nil
}
