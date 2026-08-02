package adc

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "application_default_credentials.json")
	data := []byte(`{"type":"authorized_user"}`)

	if err := Install(path, data); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Errorf("installed file contents = %q, want %q", got, data)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("installed file mode = %v, want 0600", perm)
		}
	}

	// No leftover temp files.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory contains %d entries after Install, want 1 (no leftover temp files): %v", len(entries), entries)
	}
}

func TestInstall_resolvesSymlinkDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs elevated privileges on windows")
	}
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realPath := filepath.Join(realDir, "application_default_credentials.json")
	if err := os.WriteFile(realPath, []byte("old content"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "application_default_credentials.json")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatal(err)
	}

	if err := Install(linkPath, []byte("new content")); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	// The symlink itself must still be a symlink pointing at realPath...
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("Install() replaced the symlink with a plain file, want the symlink preserved")
	}
	// ...and its target must have received the new content.
	got, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new content" {
		t.Errorf("symlink target contents = %q, want %q", got, "new content")
	}
	// No stray temp file left in the link's own directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 { // "real" dir + the symlink itself
		t.Errorf("directory contains %d entries after Install, want 2: %v", len(entries), entries)
	}
}

func TestInstall_missingSymlinkTargetFallsBackToLiteralPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "application_default_credentials.json")

	// path doesn't exist yet (the common case for a fresh install): Install
	// must not error trying to resolve a symlink that isn't there.
	if err := Install(path, []byte("content")); err != nil {
		t.Fatalf("Install() to a nonexistent path error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "content" {
		t.Errorf("installed file contents = %q, want %q", got, "content")
	}
}

func TestInstall_overwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "application_default_credentials.json")
	if err := os.WriteFile(path, []byte("old content"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Install(path, []byte("new content")); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new content" {
		t.Errorf("installed file contents = %q, want %q", got, "new content")
	}
}

func TestInstallCredential(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "application_default_credentials.json")
	c, err := Read(filepath.Join("testdata", "authorized_user.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err := InstallCredential(path, c); err != nil {
		t.Fatalf("InstallCredential() error = %v", err)
	}
	installed, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Type() != TypeAuthorizedUser {
		t.Errorf("installed credential Type() = %q, want authorized_user", installed.Type())
	}
}

func TestFilesIdentical(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	c := filepath.Join(dir, "c.json")
	if err := os.WriteFile(a, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c, []byte("different"), 0o600); err != nil {
		t.Fatal(err)
	}

	identical, err := FilesIdentical(a, b)
	if err != nil {
		t.Fatalf("FilesIdentical() error = %v", err)
	}
	if !identical {
		t.Error("FilesIdentical(a, b) = false, want true")
	}

	identical, err = FilesIdentical(a, c)
	if err != nil {
		t.Fatalf("FilesIdentical() error = %v", err)
	}
	if identical {
		t.Error("FilesIdentical(a, c) = true, want false")
	}
}
