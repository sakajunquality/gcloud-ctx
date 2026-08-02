package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sakajunquality/gcloud-ctx/internal/adc"
	"github.com/sakajunquality/gcloud-ctx/internal/gcloud"
)

// switchTo implements the switch algorithm from DESIGN.md, with the ADC
// install staged before active_config is touched at all so a late failure
// can be reported precisely:
//
//  1. validate name (NONE is always a legal target; it never has a stored
//     ADC, so it naturally skips ADC handling below and never prints the
//     no-stored-ADC hint).
//  2. read the raw active_config value as OLD; if OLD != NAME, record OLD as
//     the previous context.
//  3. if NAME has a stored ADC, parse it as a known credential type and
//     stage it into a temp file next to the live ADC (mode 0600) — before
//     touching active_config at all, so a staging failure (disk full,
//     permissions) leaves everything untouched.
//  4. activate NAME (writes active_config, touches config_sentinel).
//  5. commit: rename the staged file into place. If this fails, the error
//     names both halves of the now-inconsistent state explicitly. If the
//     stored ADC didn't parse as a known type, the switch still completes
//     and a warning (not an error) is printed instead: the live ADC is left
//     untouched. If there was no stored ADC at all, print a hint instead
//     (skipped for NONE).
//  6. print the env-var footgun warnings.
//  7. print the "Switched to" confirmation.
func (a *app) switchTo(name string) error {
	if !gcloud.IsNone(name) {
		if err := gcloud.ValidateName(name); err != nil {
			return err
		}
		exists, err := a.gs.Exists(name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("configuration %q does not exist", name)
		}
	}

	old, err := a.gs.ActiveName()
	if err != nil {
		return err
	}
	if old != name {
		if err := a.ss.SetPrevious(old); err != nil {
			return err
		}
	}

	hasADC, err := a.ss.HasADC(name)
	if err != nil {
		return err
	}

	var (
		stagedPath  string
		unparseable bool
	)
	if hasADC {
		data, err := a.ss.LoadADC(name)
		if err != nil {
			return err
		}
		cred, parseErr := adc.Parse(data)
		if parseErr != nil || !knownADCType(cred.Type()) {
			unparseable = true
		} else {
			stagedPath, err = stageFile(a.liveADCPath(), data)
			if err != nil {
				return err
			}
		}
	}

	if err := a.gs.Activate(name); err != nil {
		if stagedPath != "" {
			_ = os.Remove(stagedPath)
		}
		return err
	}

	switch {
	case unparseable:
		fmt.Fprintf(a.errw, "warning: stored ADC for %q is unparseable; live ADC unchanged (re-create it with gcloud-ctx adc save)\n", name)
	case stagedPath != "":
		if err := commitFile(a.liveADCPath(), stagedPath); err != nil {
			return fmt.Errorf("switched to %q but live ADC still belongs to the previous context: %w", name, err)
		}
	case !gcloud.IsNone(name):
		fmt.Fprintf(a.errw, "ADC unchanged (no stored ADC for %q; run 'gcloud-ctx adc save')\n", name)
	}

	warnEnv(a.errw)
	fmt.Fprintf(a.errw, "Switched to context %q.\n", name)
	return nil
}

// stageFile writes data to a temp file in the same directory as path, mode
// 0600, without installing it — the caller commits it into place later via
// commitFile. Staging first lets switchTo surface a write failure (disk
// full, permissions) before active_config is touched at all.
func stageFile(path string, data []byte) (tmpPath string, err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".adc-*.tmp")
	if err != nil {
		return "", fmt.Errorf("stage ADC file: %w", err)
	}
	tmpPath = tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("chmod staged ADC file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("write staged ADC file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("sync staged ADC file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close staged ADC file: %w", err)
	}
	return tmpPath, nil
}

// commitFile renames a file staged by stageFile into place at path,
// resolving a symlink at path the same way adc.Install does so committing
// doesn't silently replace the symlink with a plain file.
func commitFile(path, tmpPath string) error {
	resolved := path
	if target, err := filepath.EvalSymlinks(path); err == nil {
		resolved = target
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("resolve ADC destination %q: %w", path, err)
	}
	if err := os.Rename(tmpPath, resolved); err != nil {
		return fmt.Errorf("install ADC file %q: %w", resolved, err)
	}
	return nil
}

// switchToPrevious implements "gcloud-ctx -".
func (a *app) switchToPrevious() error {
	prev, err := a.ss.Previous()
	if err != nil {
		return err
	}
	if prev == "" {
		return fmt.Errorf("no previous context")
	}
	return a.switchTo(prev)
}

// rename implements "gcloud-ctx <NEW>=<OLD>" ('.' meaning the current
// context for OLD). Renaming the active context (including via '.') is
// legal: gcloud.Store.Rename itself keeps active_config coherent, and this
// just messages the result correctly.
func (a *app) rename(newName, oldName string) error {
	if oldName == "." {
		cur, err := a.gs.EffectiveActiveName()
		if err != nil {
			return err
		}
		if cur == "" {
			return fmt.Errorf("no active configuration set")
		}
		oldName = cur
	}

	result, err := a.gs.Rename(oldName, newName)
	if err != nil {
		return err
	}
	if err := a.ss.RenameADC(oldName, newName); err != nil {
		return err
	}
	if err := a.ss.RenamePreviousIfMatches(oldName, newName); err != nil {
		return err
	}

	if result.WasActive {
		fmt.Fprintf(a.errw, "Context %q renamed to %q (active).\n", oldName, newName)
	} else {
		fmt.Fprintf(a.errw, "Context %q renamed to %q.\n", oldName, newName)
	}
	if result.EnvShadowed {
		fmt.Fprintf(a.errw, "warning: %s is still set to %q; this shell's active context won't follow the rename until it's updated\n", gcloud.ActiveConfigEnvVar, oldName)
	}
	return nil
}

// deleteContexts implements "gcloud-ctx -d/--delete <NAME>...": it attempts
// every name, reporting failures inline and continuing, and reports overall
// failure (without printing again) if any name failed.
func (a *app) deleteContexts(names []string) error {
	failed := false
	for _, name := range names {
		if err := a.deleteOne(name); err != nil {
			fmt.Fprintf(a.errw, "error: %v\n", err)
			failed = true
		}
	}
	if failed {
		return errAlreadyReported
	}
	return nil
}

func (a *app) deleteOne(name string) error {
	if err := a.gs.Delete(name); err != nil {
		return err
	}
	if err := a.ss.DeleteADC(name); err != nil {
		return err
	}
	if err := a.ss.ClearPreviousIfMatches(name); err != nil {
		return err
	}
	fmt.Fprintf(a.errw, "Deleted context %q.\n", name)
	return nil
}

// printCurrent implements "gcloud-ctx -c/--current".
func (a *app) printCurrent() error {
	name, err := a.gs.EffectiveActiveName()
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("no active configuration set")
	}
	fmt.Fprintln(a.out, name)
	return nil
}
