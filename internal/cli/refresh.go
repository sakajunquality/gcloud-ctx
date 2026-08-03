package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sakajunquality/gcloud-ctx/internal/adc"
	"github.com/sakajunquality/gcloud-ctx/internal/gcloud"
)

// adcLoginRun re-obtains the base user ADC by shelling out to the real
// gcloud binary (gcloud-ctx never implements OAuth flows itself). It is a
// package variable so tests can stub the browser flow with a fake that
// writes a fresh ADC file.
var adcLoginRun = func() error {
	path, err := exec.LookPath("gcloud")
	if err != nil {
		return fmt.Errorf("gcloud not found on PATH; install the Google Cloud CLI to refresh ADC")
	}
	cmd := exec.Command(path, "auth", "application-default", "login")
	// The login flow is interactive (browser hand-off, confirmation prompt),
	// so it gets the process's real terminal, not the command's buffers.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("'gcloud auth application-default login' failed: %w", err)
	}
	return nil
}

func newRefreshCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refresh [NAME]",
		Short: "Re-authenticate ADC and rebuild every snapshot that depends on it",
		Long: strings.TrimSpace(`
Re-obtain Application Default Credentials by running 'gcloud auth
application-default login' (browser flow), then:

  1. update NAME's stored ADC snapshot (default: the current context),
     preserving its impersonation target/delegates/quota project if the
     snapshot was an impersonated one, and
  2. rebuild the stored snapshot of every other context whose credentials
     were derived from the same account — when a refresh token is revoked
     or expired (Workspace re-auth policies, password change), every copy
     of it dies at once, and this brings them all back with one login.

The live ADC file ends up matching the active context's snapshot. The
gcloud CLI's own login (credentials.db) is a separate token store; if that
is expired too, also run 'gcloud auth login'.
`),
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
			return a.refresh(name)
		},
	}
	return cmd
}

// refresh implements "gcloud-ctx refresh [NAME]".
func (a *app) refresh(context string) error {
	name, err := a.resolveImpersonateContext(context)
	if err != nil {
		return err
	}

	// Remember the target's snapshot before login so its impersonation
	// setup (if any) can be rebuilt on the fresh base afterwards.
	var oldSnap *adc.Credential
	if has, err := a.ss.HasADC(name); err != nil {
		return err
	} else if has {
		data, err := a.ss.LoadADC(name)
		if err != nil {
			return err
		}
		if oldSnap, err = adc.Parse(data); err != nil {
			return fmt.Errorf("stored ADC for %q is unparseable: %w", name, err)
		}
	}

	fmt.Fprintf(a.errw, "Running 'gcloud auth application-default login'...\n")
	if err := adcLoginRun(); err != nil {
		return err
	}

	live, err := adc.Read(a.liveADCPath())
	if err != nil {
		return fmt.Errorf("login finished but the live ADC file is unusable: %w", err)
	}
	newBase, err := adc.ResolveBase(live)
	if err != nil {
		return fmt.Errorf("login finished but the live ADC file is unusable: %w", err)
	}
	newAccount := newBase.String("account")

	if oldAccount := snapshotAccount(oldSnap); oldAccount != "" && newAccount != "" && oldAccount != newAccount {
		fmt.Fprintf(a.errw, "warning: logged in as %q but context %q previously used %q\n", newAccount, name, oldAccount)
	}

	// 1. The target context's snapshot.
	if oldSnap != nil && oldSnap.Type() == adc.TypeImpersonatedServiceAccount {
		rebuilt, err := oldSnap.WithSourceCredentials(newBase)
		if err != nil {
			return err
		}
		data, err := rebuilt.Marshal()
		if err != nil {
			return err
		}
		if err := a.ss.SaveADC(name, data); err != nil {
			return err
		}
	} else {
		data, err := live.Marshal()
		if err != nil {
			return err
		}
		if err := a.ss.SaveADC(name, data); err != nil {
			return err
		}
	}
	fmt.Fprintf(a.errw, "Refreshed ADC for context %q (account %s).\n", name, dash(newAccount))

	// 2. Cascade: rebuild every other context's snapshot derived from the
	// same account. Copies of a revoked refresh token all die together, so
	// bring them all back from the one fresh login.
	rebuilt, err := a.rebuildDependents(name, newAccount, newBase)
	if err != nil {
		return err
	}
	if len(rebuilt) > 0 {
		fmt.Fprintf(a.errw, "Rebuilt %d dependent snapshot(s): %s\n", len(rebuilt), strings.Join(rebuilt, ", "))
	}

	// 3. Leave the live ADC matching the active context's snapshot: login
	// just overwrote it with the plain base credential, which is wrong
	// whenever the active context's snapshot is an impersonated one.
	raw, err := a.gs.ActiveName()
	if err != nil {
		return err
	}
	if raw != "" && !gcloud.IsNone(raw) {
		if has, err := a.ss.HasADC(raw); err == nil && has {
			data, err := a.ss.LoadADC(raw)
			if err != nil {
				return err
			}
			if err := a.installIfActive(raw, data); err != nil {
				return fmt.Errorf("install refreshed ADC for active context %q: %w", raw, err)
			}
		}
	}
	return nil
}

// rebuildDependents rescues the snapshots of every context except target
// whose credentials were derived from account: impersonated snapshots get
// their source_credentials swapped for newBase, plain user snapshots are
// replaced by newBase outright. Returns the rebuilt context names, sorted
// (gcloud.Store.List is sorted).
func (a *app) rebuildDependents(target, account string, newBase *adc.Credential) ([]string, error) {
	if account == "" {
		return nil, nil
	}
	configs, err := a.gs.List()
	if err != nil {
		return nil, err
	}
	var rebuilt []string
	for _, cfg := range configs {
		if cfg.Name == target {
			continue
		}
		has, err := a.ss.HasADC(cfg.Name)
		if err != nil || !has {
			continue
		}
		data, err := a.ss.LoadADC(cfg.Name)
		if err != nil {
			continue
		}
		snap, err := adc.Parse(data)
		if err != nil {
			fmt.Fprintf(a.errw, "warning: stored ADC for %q is unparseable; not rebuilt\n", cfg.Name)
			continue
		}
		var fresh *adc.Credential
		switch {
		case snap.Type() == adc.TypeImpersonatedServiceAccount && snapshotAccount(snap) == account:
			if fresh, err = snap.WithSourceCredentials(newBase); err != nil {
				return rebuilt, err
			}
		case snap.Type() == adc.TypeAuthorizedUser && snap.String("account") == account:
			fresh = newBase
		default:
			continue
		}
		freshData, err := fresh.Marshal()
		if err != nil {
			return rebuilt, err
		}
		if err := a.ss.SaveADC(cfg.Name, freshData); err != nil {
			return rebuilt, err
		}
		rebuilt = append(rebuilt, cfg.Name)
	}
	return rebuilt, nil
}

// snapshotAccount returns the user account a snapshot's credentials belong
// to: the account field itself for plain user credentials, the source
// credential's account for impersonated ones, "" when undeterminable.
func snapshotAccount(c *adc.Credential) string {
	if c == nil {
		return ""
	}
	if c.Type() == adc.TypeImpersonatedServiceAccount {
		src, err := c.SourceCredentials()
		if err != nil {
			return ""
		}
		return src.String("account")
	}
	return c.String("account")
}
