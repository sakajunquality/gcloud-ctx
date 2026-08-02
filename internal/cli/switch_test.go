package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSwitch_basic(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "[core]\naccount = a@example.com\nproject = proj-a\n")
	writeFile(t, filepath.Join(dir, "configurations", "config_staging"), "[core]\naccount = b@example.com\nproject = proj-b\n")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	// Age the sentinel so we can detect the touch below.
	sentinel := filepath.Join(dir, "config_sentinel")
	writeFile(t, sentinel, "")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(sentinel, old, old); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := run(t, "staging")
	if err != nil {
		t.Fatalf("switch error = %v, stderr = %s", err, stderr)
	}

	if got := readFile(t, filepath.Join(dir, "active_config")); got != "staging" {
		t.Errorf("active_config = %q, want %q", got, "staging")
	}
	info, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().After(old) {
		t.Errorf("config_sentinel mtime not bumped: %v", info.ModTime())
	}

	if got := readFile(t, filepath.Join(dir, "gcloud-ctx", "previous")); got != "default" {
		t.Errorf("previous = %q, want %q", got, "default")
	}

	if !strings.Contains(stderr, `Switched to context "staging".`) {
		t.Errorf("stderr = %q, want Switched-to message", stderr)
	}
	if !strings.Contains(stderr, `ADC unchanged (no stored ADC for "staging"`) {
		t.Errorf("stderr = %q, want no-stored-ADC hint", stderr)
	}
}

func TestSwitch_installsStoredADC(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "configurations", "config_staging"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "staging.json"), `{"type":"authorized_user","refresh_token":"rt"}`)

	if _, _, err := run(t, "staging"); err != nil {
		t.Fatal(err)
	}

	live := filepath.Join(dir, "application_default_credentials.json")
	got := readFile(t, live)
	if !strings.Contains(got, `"refresh_token":"rt"`) {
		t.Errorf("live ADC = %q, want it to contain the stored ADC contents", got)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(live)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("live ADC mode = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestSwitch_previous(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "configurations", "config_staging"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	if _, _, err := run(t, "staging"); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := run(t, "-"); err != nil {
		t.Fatalf("switch to previous error = %v, stderr = %s", err, stderr)
	}
	if got := readFile(t, filepath.Join(dir, "active_config")); got != "default" {
		t.Errorf("active_config = %q, want default after switching back", got)
	}
}

func TestSwitch_previous_none(t *testing.T) {
	testEnv(t)
	_, _, err := run(t, "-")
	if err == nil {
		t.Fatal("switch to previous with no history: want error, got nil")
	}
}

func TestSwitch_nonexistent(t *testing.T) {
	testEnv(t)
	_, _, err := run(t, "nope")
	if err == nil {
		t.Fatal("switch to nonexistent context: want error, got nil")
	}
}

func TestSwitch_invalidName(t *testing.T) {
	testEnv(t)
	_, _, err := run(t, "Not-Valid")
	if err == nil {
		t.Fatal("switch to invalid name: want error, got nil")
	}
}

func TestUnset(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	if _, _, err := run(t, "-u"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "active_config")); got != "NONE" {
		t.Errorf("active_config = %q, want NONE", got)
	}
}

// TestSwitch_toNone_neverInstallsADC_noHint covers item 6's NONE semantics:
// switching to NONE (including via -u/--unset) must never install any ADC
// and must not print the "no stored ADC" hint either.
func TestSwitch_toNone_neverInstallsADC_noHint(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), "untouched live ADC")

	_, stderr, err := run(t, "-u")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr, "no stored ADC") {
		t.Errorf("stderr = %q, switching to NONE must not print the no-stored-ADC hint", stderr)
	}
	if got := readFile(t, filepath.Join(dir, "application_default_credentials.json")); got != "untouched live ADC" {
		t.Errorf("live ADC = %q, switching to NONE must never touch it", got)
	}
}

// TestSwitch_unparseableStoredADC_warnsButSucceeds covers the pre-install
// type check: an unparseable (or unrecognized-type) stored ADC must not
// abort the switch — it completes, and a warning is printed instead of the
// stored ADC being installed.
func TestSwitch_unparseableStoredADC_warnsButSucceeds(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "configurations", "config_staging"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "staging.json"), "not valid json at all")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), "untouched live ADC")

	_, stderr, err := run(t, "staging")
	if err != nil {
		t.Fatalf("switch with unparseable stored ADC: want success, got error = %v", err)
	}
	if !strings.Contains(stderr, `warning: stored ADC for "staging" is unparseable`) {
		t.Errorf("stderr = %q, want the unparseable-ADC warning", stderr)
	}
	if got := readFile(t, filepath.Join(dir, "application_default_credentials.json")); got != "untouched live ADC" {
		t.Errorf("live ADC = %q, want it left untouched when the stored ADC is unparseable", got)
	}
	// The switch itself must still have completed.
	if got := readFile(t, filepath.Join(dir, "active_config")); got != "staging" {
		t.Errorf("active_config = %q, want staging (switch completes despite the unparseable ADC)", got)
	}
}

// TestSwitch_lateFailure_reportsCommittedHalf covers the staged-install
// ordering: active_config is only written after the stored ADC has been
// staged, but if the final commit (rename into place) fails after Activate
// has already succeeded, the error must name both halves of the resulting
// inconsistency explicitly. A directory sitting where the live ADC file
// should go makes the final rename fail deterministically and portably
// (renaming a file over an existing directory always fails), without
// depending on directory write-permission semantics that vary by platform.
func TestSwitch_lateFailure_reportsCommittedHalf(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "configurations", "config_staging"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "staging.json"), `{"type":"authorized_user","refresh_token":"rt"}`)
	if err := writeDir(filepath.Join(dir, "application_default_credentials.json")); err != nil {
		t.Fatal(err)
	}

	_, _, err := run(t, "staging")
	if err == nil {
		t.Fatal("switch with an uncommittable live ADC destination: want error, got nil")
	}
	if !strings.Contains(err.Error(), `switched to "staging"`) || !strings.Contains(err.Error(), "live ADC still belongs to the previous context") {
		t.Errorf("error = %v, want it to explicitly name the committed half (switched, but ADC still previous)", err)
	}
	// active_config must have actually been switched despite the ADC failure.
	if got := readFile(t, filepath.Join(dir, "active_config")); got != "staging" {
		t.Errorf("active_config = %q, want staging (Activate succeeded before the failing commit)", got)
	}
}

func TestSwitch_envWarnings(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/somewhere/creds.json")

	_, stderr, err := run(t, "default")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "GOOGLE_APPLICATION_CREDENTIALS") {
		t.Errorf("stderr = %q, want GOOGLE_APPLICATION_CREDENTIALS warning", stderr)
	}
}
