package cli

import (
	"path/filepath"
	"testing"
)

func TestCurrent(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	stdout, _, err := run(t, "-c")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "default\n" {
		t.Errorf("stdout = %q, want %q", stdout, "default\n")
	}
}

func TestCurrent_noneSet(t *testing.T) {
	testEnv(t)
	if _, _, err := run(t, "-c"); err == nil {
		t.Fatal("--current with no active configuration: want error, got nil")
	}
}

func TestCurrent_envOverride(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "shadow")

	stdout, _, err := run(t, "-c")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "shadow\n" {
		t.Errorf("stdout = %q, want %q (env var wins over active_config)", stdout, "shadow\n")
	}
}
