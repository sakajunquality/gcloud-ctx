package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestShow_current(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "[core]\naccount = a@example.com\nproject = proj-a\n")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	stdout, _, err := run(t, "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "default") || !strings.Contains(stdout, "a@example.com") || !strings.Contains(stdout, "proj-a") {
		t.Errorf("stdout = %q, want context details", stdout)
	}
	if !strings.Contains(stdout, "Stored ADC:    none") {
		t.Errorf("stdout = %q, want 'Stored ADC: none' when nothing is saved", stdout)
	}
}

func TestShow_explicitName(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_other"), "[core]\naccount = b@example.com\n")

	stdout, _, err := run(t, "show", "other")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "b@example.com") {
		t.Errorf("stdout = %q, want other's properties", stdout)
	}
}

func TestShow_none(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "active_config"), "NONE")

	stdout, _, err := run(t, "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Context:       NONE") {
		t.Errorf("stdout = %q, want NONE context header", stdout)
	}
}

func TestShow_liveMatchesStored(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "default.json"), `{"type":"authorized_user"}`)
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), `{"type":"authorized_user"}`)

	stdout, _, err := run(t, "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Live ADC:      matches stored") {
		t.Errorf("stdout = %q, want live-matches-stored", stdout)
	}
}

func TestShow_noActiveConfiguration(t *testing.T) {
	testEnv(t)
	if _, _, err := run(t, "show"); err == nil {
		t.Fatal("show with no active configuration and no NAME: want error, got nil")
	}
}

// TestShow_unparseableStoredADC covers item 10: an unparseable stored ADC
// must render as "Stored ADC:    unparseable" rather than erroring out.
func TestShow_unparseableStoredADC(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "default.json"), "not valid json")

	stdout, _, err := run(t, "show")
	if err != nil {
		t.Fatalf("show with unparseable stored ADC: want success, got error = %v", err)
	}
	if !strings.Contains(stdout, "Stored ADC:    unparseable") {
		t.Errorf("stdout = %q, want 'Stored ADC:    unparseable'", stdout)
	}
}

// TestShow_delegationChain covers item 1: the raw comma chain must be split
// so the target and delegates render distinctly.
func TestShow_delegationChain(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"),
		"[auth]\nimpersonate_service_account = d1@proj.iam.gserviceaccount.com,d2@proj.iam.gserviceaccount.com,sa@proj.iam.gserviceaccount.com\n")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	stdout, _, err := run(t, "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Impersonation: sa@proj.iam.gserviceaccount.com\n") {
		t.Errorf("stdout = %q, want the target alone on the Impersonation line", stdout)
	}
	if !strings.Contains(stdout, "Delegates:     d1@proj.iam.gserviceaccount.com, d2@proj.iam.gserviceaccount.com\n") {
		t.Errorf("stdout = %q, want both delegates, in order, on the Delegates line", stdout)
	}
}

// TestShow_sanitizesControlChars covers item 9: a property value containing
// a C0 control character (which gcloud-ctx's own writes now reject, but a
// pre-existing file written by another tool might still contain) must not
// be echoed verbatim into terminal output.
func TestShow_sanitizesControlChars(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "[core]\nproject = proj\x07ect\n")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	stdout, _, err := run(t, "show")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(stdout, 0x07) {
		t.Errorf("stdout = %q, want the control character stripped", stdout)
	}
	if !strings.Contains(stdout, "project") {
		t.Errorf("stdout = %q, want the surrounding text preserved", stdout)
	}
}

// TestShow_noImpersonation_delegatesDash covers the no-delegates case: the
// Delegates line still renders, as "-", rather than being omitted.
func TestShow_noImpersonation_delegatesDash(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	stdout, _, err := run(t, "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Impersonation: -\n") {
		t.Errorf("stdout = %q, want 'Impersonation: -' when unset", stdout)
	}
	if !strings.Contains(stdout, "Delegates:     -\n") {
		t.Errorf("stdout = %q, want 'Delegates:     -' when unset", stdout)
	}
}
