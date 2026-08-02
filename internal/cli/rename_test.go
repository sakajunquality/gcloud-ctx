package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRename_basic(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_old"), "[core]\naccount = a@example.com\n")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "old.json"), `{"type":"authorized_user"}`)
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "previous"), "old")

	if _, _, err := run(t, "new=old"); err != nil {
		t.Fatal(err)
	}

	if fileExists(t, filepath.Join(dir, "configurations", "config_old")) {
		t.Error("config_old still exists after rename")
	}
	if !fileExists(t, filepath.Join(dir, "configurations", "config_new")) {
		t.Error("config_new missing after rename")
	}
	if !fileExists(t, filepath.Join(dir, "gcloud-ctx", "adc", "new.json")) {
		t.Error("stored ADC not moved to new.json")
	}
	if got := readFile(t, filepath.Join(dir, "gcloud-ctx", "previous")); got != "new" {
		t.Errorf("previous = %q, want %q (rewritten)", got, "new")
	}
}

// TestRename_active covers the internal/gcloud behavior change: renaming the
// active configuration now succeeds instead of being refused, and
// active_config is rewritten to the new name so the switch stays coherent.
func TestRename_active(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_old"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "old")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "previous"), "old")

	_, stderr, err := run(t, "new=old")
	if err != nil {
		t.Fatalf("rename of active context error = %v, want nil (must now succeed)", err)
	}
	if !fileExists(t, filepath.Join(dir, "configurations", "config_new")) {
		t.Error("config_new missing after rename")
	}
	if got := readFile(t, filepath.Join(dir, "active_config")); got != "new" {
		t.Errorf("active_config = %q, want %q (rewritten to the new name)", got, "new")
	}
	if !strings.Contains(stderr, `Context "old" renamed to "new" (active).`) {
		t.Errorf("stderr = %q, want the active-context rename message", stderr)
	}
	if got := readFile(t, filepath.Join(dir, "gcloud-ctx", "previous")); got != "new" {
		t.Errorf("previous = %q, want %q (rewritten since it pointed at the renamed active context)", got, "new")
	}
}

// TestRename_active_envShadowed covers the warning callers need when
// CLOUDSDK_ACTIVE_CONFIG_NAME still names the pre-rename context: Rename has
// no way to fix up the calling shell's environment, so the message must call
// that out explicitly rather than silently leaving the shell shadowed.
func TestRename_active_envShadowed(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_old"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "old")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "old")

	_, stderr, err := run(t, "new=old")
	if err != nil {
		t.Fatalf("rename of active context error = %v, want nil", err)
	}
	if !strings.Contains(stderr, "CLOUDSDK_ACTIVE_CONFIG_NAME") || !strings.Contains(stderr, `"old"`) {
		t.Errorf("stderr = %q, want a warning that CLOUDSDK_ACTIVE_CONFIG_NAME still names the old context", stderr)
	}
}

// TestRename_notActive_plainMessage covers the non-active rename message
// staying unadorned (no "(active)" suffix).
func TestRename_notActive_plainMessage(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_old"), "")

	_, stderr, err := run(t, "new=old")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, `Context "old" renamed to "new".`) {
		t.Errorf("stderr = %q, want the plain rename message", stderr)
	}
	if strings.Contains(stderr, "(active)") {
		t.Errorf("stderr = %q, want no '(active)' suffix for a non-active rename", stderr)
	}
}

func TestRename_dotMeansCurrent(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "default")

	// "." resolves to the *effective* active name. CLOUDSDK_ACTIVE_CONFIG_NAME
	// shadows the raw active_config file, so the rename still succeeds (it's
	// not the raw-active config), but active_config itself is untouched.
	if _, _, err := run(t, "new=."); err != nil {
		t.Fatalf("rename of current (active) context via '.': error = %v, want nil", err)
	}
	if !fileExists(t, filepath.Join(dir, "configurations", "config_new")) {
		t.Error("config_new missing after rename")
	}
}

func TestRename_dotWhenNotActive(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "configurations", "config_other"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "other")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")

	// Nothing is active per CLOUDSDK_ACTIVE_CONFIG_NAME/active_config that
	// matches "default", but EffectiveActiveName() ("other") is active, so
	// "." resolves to "other" — renaming it now succeeds and rewrites
	// active_config to follow it.
	if _, _, err := run(t, "new=."); err != nil {
		t.Fatalf("rename via '.' of the active context: error = %v, want nil", err)
	}
	if fileExists(t, filepath.Join(dir, "configurations", "config_other")) {
		t.Error("config_other still exists after rename")
	}
	if !fileExists(t, filepath.Join(dir, "configurations", "config_new")) {
		t.Error("config_new missing after rename")
	}
	if got := readFile(t, filepath.Join(dir, "active_config")); got != "new" {
		t.Errorf("active_config = %q, want %q (rewritten to follow the renamed active config)", got, "new")
	}
}

func TestRename_targetExists(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_old"), "")
	writeFile(t, filepath.Join(dir, "configurations", "config_new"), "")

	if _, _, err := run(t, "new=old"); err == nil {
		t.Fatal("rename onto an existing name: want error, got nil")
	}
}
