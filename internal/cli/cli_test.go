package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// testEnv sandboxes CLOUDSDK_CONFIG at a fresh t.TempDir() and clears the
// other env vars gcloud-ctx reads, so tests don't inherit the running
// developer's shell environment.
func testEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", dir)
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("GCLOUD_CTX_IGNORE_FZF", "1") // never launch the real fuzzy finder in tests
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	if err := os.MkdirAll(filepath.Join(dir, "configurations"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// run executes the root command with args against dir (CLOUDSDK_CONFIG must
// already be set to dir by testEnv) and returns stdout, stderr, and the
// resulting error.
func run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewRootCmd("test")
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetIn(bytes.NewReader(nil))
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeDir creates path as a directory: used to make a config/ADC path
// unwritable-as-a-file (reads/writes of it fail with "is a directory"
// instead of a plain permission error), for tests exercising a mutation
// failing partway through a multi-step operation.
func writeDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	t.Fatal(err)
	return false
}
