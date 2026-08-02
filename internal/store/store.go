// Package store manages gcloud-ctx's own state, kept under
// <gcloud config dir>/gcloud-ctx/ so that CLOUDSDK_CONFIG sandboxing (and
// tests) isolate it exactly like the rest of gcloud's config.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sakajunquality/gcloud-ctx/internal/adc"
	"github.com/sakajunquality/gcloud-ctx/internal/gcloud"
)

const (
	stateSubdir   = "gcloud-ctx"
	adcSubdir     = "adc"
	previousFile  = "previous"
	adcFileSuffix = ".json"
	dirPerm       = 0o755
	previousPerm  = 0o644
)

// Store manages gcloud-ctx's previous-context record and per-context stored
// ADC files, rooted at a gcloud config directory.
type Store struct {
	// Dir is the gcloud config directory (the same one internal/gcloud.Store
	// is rooted at).
	Dir string
}

// New returns a Store rooted at dir.
func New(dir string) *Store {
	return &Store{Dir: dir}
}

func (s *Store) baseDir() string      { return filepath.Join(s.Dir, stateSubdir) }
func (s *Store) adcDir() string       { return filepath.Join(s.baseDir(), adcSubdir) }
func (s *Store) previousPath() string { return filepath.Join(s.baseDir(), previousFile) }

// ADCPath returns the path where name's stored ADC snapshot lives, after
// validating name against gcloud's configuration name grammar (NONE
// included — it's a virtual configuration and never has a stored ADC
// snapshot). It does not check that the file exists.
func (s *Store) ADCPath(name string) (string, error) {
	if err := gcloud.ValidateName(name); err != nil {
		return "", err
	}
	dir := s.adcDir()
	path := filepath.Join(dir, name+adcFileSuffix)
	// Defense in depth: ValidateName's regex already rules out "/" and ".."
	// in name, so this can't actually trigger, but confirm the join didn't
	// escape dir anyway rather than trusting the regex alone.
	if filepath.Dir(path) != filepath.Clean(dir) {
		return "", fmt.Errorf("invalid configuration name %q", name)
	}
	return path, nil
}

// Previous returns the previous context name, or "" if none is recorded.
func (s *Store) Previous() (string, error) {
	data, err := os.ReadFile(s.previousPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

// SetPrevious records name as the previous context (for "gcloud-ctx -").
func (s *Store) SetPrevious(name string) error {
	if err := os.MkdirAll(s.baseDir(), dirPerm); err != nil {
		return err
	}
	return os.WriteFile(s.previousPath(), []byte(name), previousPerm)
}

// ClearPrevious removes the previous-context record entirely.
func (s *Store) ClearPrevious() error {
	err := os.Remove(s.previousPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ClearPreviousIfMatches clears the previous-context record only if it
// currently points at name (used when deleting a context).
func (s *Store) ClearPreviousIfMatches(name string) error {
	cur, err := s.Previous()
	if err != nil {
		return err
	}
	if cur != name {
		return nil
	}
	return s.ClearPrevious()
}

// RenamePreviousIfMatches updates the previous-context record from oldName
// to newName, but only if it currently points at oldName (used when renaming
// a context).
func (s *Store) RenamePreviousIfMatches(oldName, newName string) error {
	cur, err := s.Previous()
	if err != nil {
		return err
	}
	if cur != oldName {
		return nil
	}
	return s.SetPrevious(newName)
}

// HasADC reports whether name has a stored ADC snapshot. NONE always
// reports false without error: it's gcloud's virtual, file-less
// configuration and can never have one (unlike gcloud.Store.Exists, which
// reports NONE as always existing — there's no equivalent virtual ADC
// snapshot to report as present).
func (s *Store) HasADC(name string) (bool, error) {
	if gcloud.IsNone(name) {
		return false, nil
	}
	path, err := s.ADCPath(name)
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

// LoadADC reads name's stored ADC snapshot.
func (s *Store) LoadADC(name string) ([]byte, error) {
	path, err := s.ADCPath(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// SaveADC atomically stores data as name's ADC snapshot (mode 0600). It's the
// explicit "gcloud-ctx adc save" / "impersonate" write path — there is no
// automatic snapshotting.
func (s *Store) SaveADC(name string, data []byte) error {
	path, err := s.ADCPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.adcDir(), dirPerm); err != nil {
		return err
	}
	return adc.Install(path, data)
}

// DeleteADC removes name's stored ADC snapshot, if any.
func (s *Store) DeleteADC(name string) error {
	path, err := s.ADCPath(name)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// RenameADC moves oldName's stored ADC snapshot to newName, if present. It's
// a no-op (not an error) if oldName has no stored ADC — but newName is still
// validated even in that case, so callers get a clear error rather than
// silently accepting a bad target name.
func (s *Store) RenameADC(oldName, newName string) error {
	// Validated explicitly (rather than relying solely on the HasADC call
	// below): HasADC special-cases NONE to a graceful "false, nil" for the
	// switch-to-NONE path, but a rename attempt naming NONE as the source
	// must still be a hard error, not a silent no-op.
	if err := gcloud.ValidateName(oldName); err != nil {
		return err
	}
	newPath, err := s.ADCPath(newName)
	if err != nil {
		return err
	}
	has, err := s.HasADC(oldName)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	oldPath, err := s.ADCPath(oldName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.adcDir(), dirPerm); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}
