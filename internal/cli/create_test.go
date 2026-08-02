package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCreate_activatesByDefault(t *testing.T) {
	dir := testEnv(t)

	if _, _, err := run(t, "create", "staging", "--project", "proj-a", "--account", "a@example.com"); err != nil {
		t.Fatal(err)
	}

	ini := readFile(t, filepath.Join(dir, "configurations", "config_staging"))
	if !strings.Contains(ini, "proj-a") || !strings.Contains(ini, "a@example.com") {
		t.Errorf("config_staging = %q, want properties set", ini)
	}
	if got := readFile(t, filepath.Join(dir, "active_config")); got != "staging" {
		t.Errorf("active_config = %q, want staging (activated by default)", got)
	}
}

func TestCreate_noActivate(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	if _, _, err := run(t, "create", "staging", "--no-activate"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "active_config")); got != "default" {
		t.Errorf("active_config = %q, want default unchanged (--no-activate)", got)
	}
}

func TestCreate_refusesExisting(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")

	if _, _, err := run(t, "create", "default"); err == nil {
		t.Fatal("create of an existing name: want error, got nil")
	}
}

func TestCreate_refusesNone(t *testing.T) {
	testEnv(t)
	if _, _, err := run(t, "create", "NONE"); err == nil {
		t.Fatal("create of NONE: want error, got nil")
	}
}

// TestCreate_activateDefault_singleADCHint covers item 11: activating by
// default must print exactly one ADC hint line (switchTo's own "no stored
// ADC" hint), not create's own hint too — the two used to be redundant.
func TestCreate_activateDefault_singleADCHint(t *testing.T) {
	testEnv(t)

	_, stderr, err := run(t, "create", "staging")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(stderr, "ADC unchanged"); n != 1 {
		t.Errorf("stderr = %q, want exactly one ADC-unchanged hint line, got %d", stderr, n)
	}
}

// TestCreate_noActivate_hasADCHint covers the --no-activate path, where
// switchTo never runs: create's own ADC hint must still appear (exactly
// once) so the user isn't left with no guidance at all.
func TestCreate_noActivate_hasADCHint(t *testing.T) {
	testEnv(t)

	_, stderr, err := run(t, "create", "staging", "--no-activate")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(stderr, "ADC unchanged"); n != 1 {
		t.Errorf("stderr = %q, want exactly one ADC-unchanged hint line, got %d", stderr, n)
	}
}
