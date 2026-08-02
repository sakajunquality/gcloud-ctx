package cli

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sakajunquality/gcloud-ctx/internal/adc"
	"github.com/sakajunquality/gcloud-ctx/internal/gcloud"
)

// serviceAccountRE matches a syntactically valid service account email:
// strict enough to reject '/', ':', '?', '#', spaces, and newlines from ever
// reaching the INI file or an impersonation URL.
var serviceAccountRE = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.gserviceaccount\.com$`)

// validateServiceAccount reports whether sa is a syntactically valid service
// account email ending in .gserviceaccount.com.
func validateServiceAccount(sa string) error {
	if !serviceAccountRE.MatchString(sa) {
		return fmt.Errorf("invalid service account %q: must match %s", sa, serviceAccountRE.String())
	}
	return nil
}

// splitImpersonationChain splits the raw auth/impersonate_service_account
// property value into its target service account and delegation chain,
// matching gcloud's own comma-chain encoding (ParseImpersonationAccounts):
// delegates first, target last. A property with no delegates is just the
// bare target SA.
func splitImpersonationChain(raw string) (target string, delegates []string) {
	if raw == "" {
		return "", nil
	}
	parts := strings.Split(raw, ",")
	return parts[len(parts)-1], parts[:len(parts)-1]
}

// joinImpersonationChain builds the raw auth/impersonate_service_account
// property value for target with delegates in order, target last — the
// inverse of splitImpersonationChain.
func joinImpersonationChain(target string, delegates []string) string {
	chain := make([]string, 0, len(delegates)+1)
	chain = append(chain, delegates...)
	chain = append(chain, target)
	return strings.Join(chain, ",")
}

func newImpersonateCmd() *cobra.Command {
	var (
		delegates    []string
		quotaProject string
		context      string
		clearFlag    bool
	)

	cmd := &cobra.Command{
		Use:   "impersonate [SERVICE_ACCOUNT]",
		Short: "Set up (or clear) service account impersonation for a context",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp(cmd)
			if err != nil {
				return err
			}
			if clearFlag {
				if len(args) != 0 {
					return fmt.Errorf("--clear takes no arguments")
				}
				return a.impersonateClear(context)
			}
			if len(args) != 1 {
				return fmt.Errorf("accepts 1 arg (the target service account), received %d", len(args))
			}
			return a.impersonateSet(context, args[0], delegates, quotaProject)
		},
	}

	cmd.Flags().StringSliceVar(&delegates, "delegates", nil, "delegation chain toward the target service account, in order")
	cmd.Flags().StringVar(&quotaProject, "quota-project", "", "quota project to record on the impersonated ADC")
	cmd.Flags().StringVar(&context, "context", "", "target context (default: current)")
	cmd.Flags().BoolVar(&clearFlag, "clear", false, "clear impersonation instead of setting it")

	return cmd
}

// resolveImpersonateContext resolves the target context name for
// impersonate: the --context flag if given, else the current context. NONE
// is never a legal target — it has no INI file to write into.
func (a *app) resolveImpersonateContext(context string) (string, error) {
	name := context
	if name == "" {
		eff, err := a.gs.EffectiveActiveName()
		if err != nil {
			return "", err
		}
		if eff == "" {
			return "", fmt.Errorf("no active configuration set; pass --context")
		}
		name = eff
	}
	if gcloud.IsNone(name) {
		return "", fmt.Errorf("cannot impersonate for the %q configuration", gcloud.NoneConfig)
	}
	exists, err := a.gs.Exists(name)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("configuration %q does not exist", name)
	}
	return name, nil
}

// resolveBaseCredential determines the base credentials to impersonate on
// top of per DESIGN.md: the context's stored ADC if present, else the live
// ADC; unwrapped and validated via adc.ResolveBase. Falling back to the live
// ADC (because name has no stored snapshot of its own) is called out on
// stderr: the live ADC belongs to whatever context is actually active, which
// may not be name — most notably with "impersonate --context OTHER".
func (a *app) resolveBaseCredential(name string) (*adc.Credential, error) {
	hasADC, err := a.ss.HasADC(name)
	if err != nil {
		return nil, err
	}

	var data []byte
	if hasADC {
		data, err = a.ss.LoadADC(name)
		if err != nil {
			return nil, err
		}
	} else {
		data, err = os.ReadFile(a.liveADCPath())
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no stored ADC for %q and no live ADC file; run 'gcloud auth application-default login' first", name)
		}
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(a.errw, "using current live ADC (account %s) as base credentials for context %q\n", a.currentAccountHint(), name)
	}

	cred, err := adc.Parse(data)
	if err != nil {
		return nil, err
	}
	return adc.ResolveBase(cred)
}

// currentAccountHint returns the raw active context's core/account, or
// "unknown" if it can't be determined. It's used only for the informational
// notice printed when impersonate falls back to the live ADC as base
// credentials, to make clear whose credentials are being borrowed.
func (a *app) currentAccountHint() string {
	raw, err := a.gs.ActiveName()
	if err != nil || raw == "" || gcloud.IsNone(raw) {
		return "unknown"
	}
	props, err := a.gs.GetProperties(raw)
	if err != nil || props.Account == "" {
		return "unknown"
	}
	return props.Account
}

// installIfActive installs data as the live ADC file, but only if name is
// the raw active configuration (not just effectively active via an env-var
// override) — matching the check DESIGN.md specifies for impersonate.
func (a *app) installIfActive(name string, data []byte) error {
	raw, err := a.gs.ActiveName()
	if err != nil {
		return err
	}
	if raw != name {
		return nil
	}
	return adc.Install(a.liveADCPath(), data)
}

func (a *app) impersonateSet(context, sa string, delegates []string, quotaProject string) error {
	name, err := a.resolveImpersonateContext(context)
	if err != nil {
		return err
	}
	if err := validateServiceAccount(sa); err != nil {
		return err
	}
	for _, d := range delegates {
		if err := validateServiceAccount(d); err != nil {
			return fmt.Errorf("invalid delegate: %w", err)
		}
	}

	base, err := a.resolveBaseCredential(name)
	if err != nil {
		return err
	}

	cred, err := adc.Synthesize(adc.ImpersonateOptions{
		ServiceAccount: sa,
		Delegates:      delegates,
		QuotaProject:   quotaProject,
		Source:         base,
	})
	if err != nil {
		return err
	}
	data, err := cred.Marshal()
	if err != nil {
		return err
	}

	// ADC (the store snapshot and, if applicable, the live install) is
	// written before the gcloud-native property: if the property write then
	// fails, the caller needs to know ADC already moved.
	if err := a.ss.SaveADC(name, data); err != nil {
		return err
	}
	if err := a.installIfActive(name, data); err != nil {
		return err
	}

	chain := joinImpersonationChain(sa, delegates)
	if err := a.gs.SetProperty(name, "auth", "impersonate_service_account", chain); err != nil {
		return fmt.Errorf("ADC updated for %q but the gcloud property was not set: %w", name, err)
	}

	fmt.Fprintf(a.errw, "Context %q now impersonates %q.\n", name, sa)
	return nil
}

func (a *app) impersonateClear(context string) error {
	name, err := a.resolveImpersonateContext(context)
	if err != nil {
		return err
	}

	hasADC, err := a.ss.HasADC(name)
	if err != nil {
		return err
	}
	if hasADC {
		data, err := a.ss.LoadADC(name)
		if err != nil {
			return err
		}
		cred, err := adc.Parse(data)
		if err != nil {
			return err
		}
		if cred.Type() == adc.TypeImpersonatedServiceAccount {
			unwrapped, err := adc.Unimpersonate(cred)
			if err != nil {
				return err
			}
			out, err := unwrapped.Marshal()
			if err != nil {
				return err
			}
			if err := a.ss.SaveADC(name, out); err != nil {
				return err
			}
			if err := a.installIfActive(name, out); err != nil {
				return err
			}
		}
	}

	// Beyond the stored-snapshot path above, if name is the raw-active
	// context the live ADC file is what gcloud and client libraries actually
	// read, and it may be impersonating even when there's no stored snapshot
	// reflecting that (or gcloud-ctx's own store doesn't know about it).
	// Unwrap it directly too.
	raw, err := a.gs.ActiveName()
	if err != nil {
		return err
	}
	if raw == name {
		a.clearLiveImpersonation()
	}

	if err := a.gs.DeleteProperty(name, "auth", "impersonate_service_account"); err != nil {
		return fmt.Errorf("ADC updated for %q but the gcloud property was not cleared: %w", name, err)
	}

	fmt.Fprintf(a.errw, "Context %q impersonation cleared.\n", name)
	return nil
}

// clearLiveImpersonation reads the live ADC file directly and, if it is
// impersonated_service_account, unwraps and atomically installs the result.
// It never fails the overall clear operation: if the live ADC can't be read,
// parsed, or unwrapped, it prints a warning to stderr instead, since the INI
// property (the gcloud-native impersonation switch) still gets cleared
// regardless of what the live ADC file contains.
func (a *app) clearLiveImpersonation() {
	data, err := os.ReadFile(a.liveADCPath())
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		fmt.Fprintf(a.errw, "warning: could not read live ADC (%v); it may still impersonate — reset it with 'gcloud auth application-default login'\n", err)
		return
	}
	cred, err := adc.Parse(data)
	if err != nil {
		fmt.Fprintf(a.errw, "warning: live ADC still impersonates (could not parse it to unwrap); reset it with 'gcloud auth application-default login'\n")
		return
	}
	if cred.Type() != adc.TypeImpersonatedServiceAccount {
		return
	}
	unwrapped, err := adc.Unimpersonate(cred)
	if err != nil {
		fmt.Fprintf(a.errw, "warning: live ADC still impersonates (%v); reset it with 'gcloud auth application-default login'\n", err)
		return
	}
	out, err := unwrapped.Marshal()
	if err != nil {
		fmt.Fprintf(a.errw, "warning: live ADC still impersonates (%v); reset it with 'gcloud auth application-default login'\n", err)
		return
	}
	if err := adc.Install(a.liveADCPath(), out); err != nil {
		fmt.Fprintf(a.errw, "warning: live ADC still impersonates (could not install unwrapped credentials: %v); reset it with 'gcloud auth application-default login'\n", err)
		return
	}
}
