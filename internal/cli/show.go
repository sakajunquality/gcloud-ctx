package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sakajunquality/gcloud-ctx/internal/adc"
	"github.com/sakajunquality/gcloud-ctx/internal/gcloud"
)

func newShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "show [NAME]",
		Short:             "Show context details, including impersonation and ADC status",
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
			return a.show(name)
		},
	}
	return cmd
}

// show implements "gcloud-ctx show [NAME]": defaults NAME to the current
// context, and prints properties, impersonation target, and stored-vs-live
// ADC status.
func (a *app) show(name string) error {
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

	fmt.Fprintf(a.out, "Context:       %s\n", name)

	if gcloud.IsNone(name) {
		fmt.Fprintln(a.out, "Account:       -")
		fmt.Fprintln(a.out, "Project:       -")
		fmt.Fprintln(a.out, "Region:        -")
		fmt.Fprintln(a.out, "Zone:          -")
		fmt.Fprintln(a.out, "Impersonation: -")
		fmt.Fprintln(a.out, "Stored ADC:    none")
		warnEnv(a.errw)
		return nil
	}

	exists, err := a.gs.Exists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("configuration %q does not exist", name)
	}

	props, err := a.gs.GetProperties(name)
	if err != nil {
		return err
	}
	target, delegates := splitImpersonationChain(props.ImpersonateServiceAccount)
	fmt.Fprintf(a.out, "Account:       %s\n", dash(sanitize(props.Account)))
	fmt.Fprintf(a.out, "Project:       %s\n", dash(sanitize(props.Project)))
	fmt.Fprintf(a.out, "Region:        %s\n", dash(sanitize(props.Region)))
	fmt.Fprintf(a.out, "Zone:          %s\n", dash(sanitize(props.Zone)))
	fmt.Fprintf(a.out, "Impersonation: %s\n", dash(sanitize(target)))
	fmt.Fprintf(a.out, "Delegates:     %s\n", dash(sanitize(strings.Join(delegates, ", "))))

	if err := a.showADCStatus(name); err != nil {
		return err
	}

	warnEnv(a.errw)
	return nil
}

func (a *app) showADCStatus(name string) error {
	hasADC, err := a.ss.HasADC(name)
	if err != nil {
		return err
	}
	if !hasADC {
		fmt.Fprintln(a.out, "Stored ADC:    none")
		return nil
	}

	data, err := a.ss.LoadADC(name)
	if err != nil {
		return err
	}
	cred, err := adc.Parse(data)
	if err != nil {
		fmt.Fprintln(a.out, "Stored ADC:    unparseable")
		return nil
	}
	fmt.Fprintf(a.out, "Stored ADC:    %s\n", cred.Type())
	if cred.Type() == adc.TypeImpersonatedServiceAccount {
		if target, err := cred.TargetServiceAccount(); err == nil {
			fmt.Fprintf(a.out, "Impersonating: %s\n", target)
		}
	}

	if _, err := os.Stat(a.liveADCPath()); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(a.out, "Live ADC:      not present")
		return nil
	} else if err != nil {
		return err
	}

	storedADCPath, err := a.ss.ADCPath(name)
	if err != nil {
		return err
	}
	identical, err := adc.FilesIdentical(a.liveADCPath(), storedADCPath)
	if err != nil {
		return err
	}
	if identical {
		fmt.Fprintln(a.out, "Live ADC:      matches stored")
	} else {
		fmt.Fprintln(a.out, "Live ADC:      differs from stored")
	}
	return nil
}
