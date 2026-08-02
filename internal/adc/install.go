package adc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Install atomically writes data to path: a temp file is created in the same
// directory with mode 0600, fsynced and renamed into place. ADC files are
// credentials — this is the only way gcloud-ctx ever writes one.
//
// If path itself is a symlink, the temp file is created alongside (and the
// rename lands on) its resolved target instead of the link, so installing
// doesn't silently replace the symlink with a plain file. A path that
// doesn't exist yet (the common case — there's nothing to resolve) falls
// back to the literal path.
func Install(path string, data []byte) error {
	resolved := path
	if target, err := filepath.EvalSymlinks(path); err == nil {
		resolved = target
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("resolve ADC destination %q: %w", path, err)
	}

	dir := filepath.Dir(resolved)
	tmp, err := os.CreateTemp(dir, ".adc-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp ADC file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once the rename below succeeds

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp ADC file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp ADC file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp ADC file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp ADC file: %w", err)
	}
	if err := os.Rename(tmpPath, resolved); err != nil {
		return fmt.Errorf("install ADC file %q: %w", resolved, err)
	}
	return nil
}

// InstallCredential marshals c and installs it at path.
func InstallCredential(path string, c *Credential) error {
	data, err := c.Marshal()
	if err != nil {
		return err
	}
	return Install(path, data)
}

// FilesIdentical reports whether the files at a and b have byte-identical
// contents. It's used to tell whether the live ADC file matches a stored
// one.
func FilesIdentical(a, b string) (bool, error) {
	da, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(da, db), nil
}
