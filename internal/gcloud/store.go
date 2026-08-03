package gcloud

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/ini.v1"
)

const (
	configurationsSubdir = "configurations"
	configFilePrefix     = "config_"
	activeConfigFile     = "active_config"
	configSentinelFile   = "config_sentinel"
)

// ActiveConfigEnvVar is the environment variable that shadows the
// active_config file (gcloud has no --configuration flag equivalent here).
const ActiveConfigEnvVar = "CLOUDSDK_ACTIVE_CONFIG_NAME"

// Store manages the named configurations under a gcloud config directory
// (as resolved by ConfigDir).
type Store struct {
	// Dir is the gcloud config directory, e.g. $HOME/.config/gcloud.
	Dir string
}

// NewStore returns a Store rooted at dir.
func NewStore(dir string) *Store {
	return &Store{Dir: dir}
}

// Open resolves the gcloud config directory via ConfigDir and returns a
// Store for it.
func Open() (*Store, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	return NewStore(dir), nil
}

func (s *Store) configurationsDir() string {
	return filepath.Join(s.Dir, configurationsSubdir)
}

// ConfigPath returns the path to the named configuration's INI file, after
// validating name against the configuration name grammar (NONE included —
// it's virtual and has no backing file). It does not check that the file
// exists.
func (s *Store) ConfigPath(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	dir := s.configurationsDir()
	path := filepath.Join(dir, configFilePrefix+name)
	// Defense in depth: ValidateName's regex already rules out "/" and ".."
	// in name, so this can't actually trigger, but confirm the join didn't
	// escape dir anyway rather than trusting the regex alone.
	if filepath.Dir(path) != filepath.Clean(dir) {
		return "", fmt.Errorf("invalid configuration name %q", name)
	}
	return path, nil
}

// Exists reports whether a named configuration has a backing file. NONE
// always reports true, since it's a virtual configuration that always
// exists.
func (s *Store) Exists(name string) (bool, error) {
	if IsNone(name) {
		return true, nil
	}
	path, err := s.ConfigPath(name)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// List returns every named configuration, sorted by name, with properties
// parsed from each backing INI file. A configuration file that fails to
// parse does not abort the listing: it's included with a zero-value
// Properties and ParseErr set, so callers can still see (and report on) it.
func (s *Store) List() ([]Config, error) {
	entries, err := os.ReadDir(s.configurationsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read configurations directory: %w", err)
	}

	var configs []Config
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name, ok := strings.CutPrefix(e.Name(), configFilePrefix)
		if !ok || ValidateName(name) != nil {
			continue // not a config_<name> file, or not a well-formed name
		}
		props, err := s.GetProperties(name)
		if err != nil {
			configs = append(configs, Config{Name: name, ParseErr: err})
			continue
		}
		configs = append(configs, Config{Name: name, Properties: props})
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].Name < configs[j].Name })
	return configs, nil
}

// ActiveName returns the raw contents of the active_config file (the bare
// configuration name, no trailing newline), or "" if the file doesn't exist.
// It does not consult CLOUDSDK_ACTIVE_CONFIG_NAME; see EffectiveActiveName.
func (s *Store) ActiveName() (string, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, activeConfigFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read active_config: %w", err)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

// EffectiveActiveName returns the configuration that would actually be used:
// CLOUDSDK_ACTIVE_CONFIG_NAME if set (even if it names a configuration with
// no backing file — that's valid, not corruption), else ActiveName.
func (s *Store) EffectiveActiveName() (string, error) {
	if v := os.Getenv(ActiveConfigEnvVar); v != "" {
		return v, nil
	}
	return s.ActiveName()
}

// IsActive reports whether name is the effective active configuration.
func (s *Store) IsActive(name string) (bool, error) {
	active, err := s.EffectiveActiveName()
	if err != nil {
		return false, err
	}
	return active == name, nil
}

// Activate switches the active configuration to name: it writes
// active_config with the exact bytes of name (no trailing newline) and
// touches config_sentinel so IDE plugins/credential helpers re-read config.
// name must be NoneConfig or an existing configuration.
func (s *Store) Activate(name string) error {
	if !IsNone(name) {
		if err := ValidateName(name); err != nil {
			return err
		}
		exists, err := s.Exists(name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("configuration %q does not exist", name)
		}
	}

	if err := s.writeActiveConfig(name); err != nil {
		return err
	}
	return s.touchSentinel()
}

// writeActiveConfig overwrites active_config with the exact bytes of name
// (no trailing newline), creating the config directory if needed.
func (s *Store) writeActiveConfig(name string) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, activeConfigFile), []byte(name), 0o644); err != nil {
		return fmt.Errorf("write active_config: %w", err)
	}
	return nil
}

// touchSentinel bumps config_sentinel's mtime (creating it if absent) so
// IDE plugins/credential helpers know something changed. It's called on
// every config mutation (Activate, Create, Delete, Rename, SetProperty,
// DeleteProperty) — matching gcloud, which touches it any time there is a
// change to config, not only on activation.
func (s *Store) touchSentinel() error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := touch(filepath.Join(s.Dir, configSentinelFile)); err != nil {
		return fmt.Errorf("touch config_sentinel: %w", err)
	}
	return nil
}

// touch updates the mtime of path, creating an empty file if it doesn't
// already exist.
func touch(path string) error {
	now := time.Now()
	if err := os.Chtimes(path, now, now); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// Create makes a new named configuration with the given properties set (zero
// fields are left unset). It refuses NONE and names that already exist. It
// does not activate the configuration or touch ADC; callers compose that.
func (s *Store) Create(name string, props Properties) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	// Validate every property value up front so a bad one is refused before
	// anything is written to disk, rather than leaving a half-created
	// (empty) config file behind.
	if err := validateProperties(props); err != nil {
		return err
	}
	exists, err := s.Exists(name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("configuration %q already exists", name)
	}

	path, err := s.ConfigPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create configurations directory: %w", err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return fmt.Errorf("create configuration %q: %w", name, err)
	}
	if err := s.applyProperties(name, props); err != nil {
		return err
	}
	return s.touchSentinel()
}

// validateProperties validates every set field of props via
// validatePropertyValue.
func validateProperties(props Properties) error {
	for _, v := range []string{props.Account, props.Project, props.Region, props.Zone, props.ImpersonateServiceAccount} {
		if v == "" {
			continue
		}
		if err := validatePropertyValue(v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyProperties(name string, props Properties) error {
	return s.mutateINI(name, func(f *ini.File) error {
		set := func(section, key, value string) error {
			if value == "" {
				return nil
			}
			if err := validatePropertyValue(value); err != nil {
				return err
			}
			f.Section(section).Key(key).SetValue(value)
			return nil
		}
		if err := set("core", "account", props.Account); err != nil {
			return err
		}
		if err := set("core", "project", props.Project); err != nil {
			return err
		}
		if err := set("compute", "region", props.Region); err != nil {
			return err
		}
		if err := set("compute", "zone", props.Zone); err != nil {
			return err
		}
		if err := set("auth", "impersonate_service_account", props.ImpersonateServiceAccount); err != nil {
			return err
		}
		return nil
	})
}

// RenameResult reports what Store.Rename actually did to the active-config
// bookkeeping, so callers can message the user correctly.
type RenameResult struct {
	// WasActive reports whether oldName was the raw active_config value.
	// If true, Rename already rewrote active_config to newName and touched
	// config_sentinel as part of the rename.
	WasActive bool
	// EnvShadowed reports whether CLOUDSDK_ACTIVE_CONFIG_NAME equaled
	// oldName. Rename does not (cannot) update the calling shell's
	// environment, so callers should warn that this shell still resolves
	// to the old name until the env var is updated.
	EnvShadowed bool
}

// Rename renames configuration oldName to newName. It refuses NONE as
// oldName (never a legal rename source — it has no backing file), a
// nonexistent oldName, and a newName that's invalid or already taken.
//
// Unlike gcloud's own "configurations rename", renaming the active
// configuration is allowed: the file is renamed first, then active_config is
// rewritten to newName and config_sentinel is touched, so the active
// configuration stays coherent throughout. See RenameResult for what the
// caller needs in order to message this (and the env-var-shadow case, which
// Rename proceeds through since it has no way to fix up the calling shell's
// environment).
func (s *Store) Rename(oldName, newName string) (RenameResult, error) {
	if err := ValidateName(oldName); err != nil {
		return RenameResult{}, err
	}
	if err := ValidateName(newName); err != nil {
		return RenameResult{}, err
	}

	oldPath, err := s.ConfigPath(oldName)
	if err != nil {
		return RenameResult{}, err
	}
	if _, err := os.Stat(oldPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RenameResult{}, fmt.Errorf("configuration %q does not exist", oldName)
		}
		return RenameResult{}, err
	}
	newExists, err := s.Exists(newName)
	if err != nil {
		return RenameResult{}, err
	}
	if newExists {
		return RenameResult{}, fmt.Errorf("configuration %q already exists", newName)
	}
	newPath, err := s.ConfigPath(newName)
	if err != nil {
		return RenameResult{}, err
	}

	raw, err := s.ActiveName()
	if err != nil {
		return RenameResult{}, err
	}
	result := RenameResult{
		WasActive:   oldName == raw,
		EnvShadowed: os.Getenv(ActiveConfigEnvVar) == oldName,
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return RenameResult{}, fmt.Errorf("rename configuration %q to %q: %w", oldName, newName, err)
	}

	if result.WasActive {
		if err := s.writeActiveConfig(newName); err != nil {
			return result, err
		}
	}
	if err := s.touchSentinel(); err != nil {
		return result, err
	}
	return result, nil
}

// Delete removes configuration name. It refuses NONE and the active
// configuration (checked against both the raw active_config file and the
// effective active name).
func (s *Store) Delete(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if err := s.guardNotActive(name, "delete"); err != nil {
		return err
	}

	path, err := s.ConfigPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("configuration %q does not exist", name)
		}
		return fmt.Errorf("delete configuration %q: %w", name, err)
	}
	return s.touchSentinel()
}

// guardNotActive refuses op if name is the active configuration per either
// the raw active_config file or the effective (env-var aware) active name —
// gcloud's own double check.
func (s *Store) guardNotActive(name, op string) error {
	raw, err := s.ActiveName()
	if err != nil {
		return err
	}
	effective, err := s.EffectiveActiveName()
	if err != nil {
		return err
	}
	if name == raw || name == effective {
		return fmt.Errorf("cannot %s active configuration %q", op, name)
	}
	return nil
}

// GetProperties reads the recognized properties from the named
// configuration's INI file. A missing or empty file yields zero-value
// Properties (empty file is a valid, brand-new configuration).
func (s *Store) GetProperties(name string) (Properties, error) {
	f, err := s.loadINI(name)
	if err != nil {
		return Properties{}, err
	}
	return Properties{
		Account:                   f.Section("core").Key("account").String(),
		Project:                   f.Section("core").Key("project").String(),
		Region:                    f.Section("compute").Key("region").String(),
		Zone:                      f.Section("compute").Key("zone").String(),
		ImpersonateServiceAccount: f.Section("auth").Key("impersonate_service_account").String(),
	}, nil
}

// validatePropertyValue rejects property values that could corrupt the INI
// file or smuggle extra keys/lines into it: CR, LF, and any other C0
// control character (0x00-0x1F, which already includes both).
func validatePropertyValue(value string) error {
	for _, r := range value {
		if r < 0x20 {
			return fmt.Errorf("invalid property value %q: contains control character %U", value, r)
		}
	}
	return nil
}

// SetProperty sets section/key to value in the named configuration's INI
// file, preserving unrelated sections/keys/comments (load-mutate-save). The
// configuration file must already exist. value must not contain newlines or
// other control characters.
func (s *Store) SetProperty(name, section, key, value string) error {
	if err := validatePropertyValue(value); err != nil {
		return err
	}
	if err := s.mutateINI(name, func(f *ini.File) error {
		f.Section(section).Key(key).SetValue(value)
		return nil
	}); err != nil {
		return err
	}
	return s.touchSentinel()
}

// DeleteProperty removes section/key from the named configuration's INI
// file. If the section becomes empty as a result, the section itself is
// dropped too. It is not an error for the key or section to already be
// absent.
func (s *Store) DeleteProperty(name, section, key string) error {
	if err := s.mutateINI(name, func(f *ini.File) error {
		if !f.HasSection(section) {
			return nil
		}
		sec, err := f.GetSection(section)
		if err != nil {
			return err
		}
		sec.DeleteKey(key)
		if len(sec.Keys()) == 0 {
			f.DeleteSection(section)
		}
		return nil
	}); err != nil {
		return err
	}
	return s.touchSentinel()
}

// iniLoadOptions governs every INI parse in this package.
//
// IgnoreInlineComment keeps a '#' or ';' occurring inside a value as part of
// that value instead of silently truncating it there — gcloud config values
// don't need this themselves, but a value written by some other tool that
// does contain one must survive a read-modify-write round trip unchanged.
//
// AllowPythonMultilineValues accepts configparser-style indented
// continuation lines (a value spanning multiple lines, each indented deeper
// than its key) instead of erroring out on them, so List/SetProperty don't
// abort on a config file gcloud or another tool wrote that way.
//
// Note: gopkg.in/ini.v1 unconditionally strips a value's surrounding double
// quotes on parse, with no cheap way to tell `key = "v"` apart from
// `key = v` afterward — gcloud-ctx does not attempt to preserve that
// quoting on round trip as a result.
var iniLoadOptions = ini.LoadOptions{
	IgnoreInlineComment:        true,
	AllowPythonMultilineValues: true,
}

func (s *Store) loadINI(name string) (*ini.File, error) {
	path, err := s.ConfigPath(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("configuration %q does not exist", name)
		}
		return nil, fmt.Errorf("read configuration %q: %w", name, err)
	}
	f, err := ini.LoadSources(iniLoadOptions, data)
	if err != nil {
		return nil, fmt.Errorf("parse configuration %q: %w", name, err)
	}
	return f, nil
}

// mutateINI loads the named configuration's INI file (an absent file is
// treated as empty, so property writes can create the backing file), applies
// fn, and writes the whole file back — preserving any sections/keys/comments
// fn didn't touch.
func (s *Store) mutateINI(name string, fn func(*ini.File) error) error {
	path, err := s.ConfigPath(name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read configuration %q: %w", name, err)
		}
		data = nil
	}

	f, err := ini.LoadSources(iniLoadOptions, data)
	if err != nil {
		return fmt.Errorf("parse configuration %q: %w", name, err)
	}
	if err := fn(f); err != nil {
		return err
	}

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return fmt.Errorf("render configuration %q: %w", name, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create configurations directory: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write configuration %q: %w", name, err)
	}
	return nil
}
