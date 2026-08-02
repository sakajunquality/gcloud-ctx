package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEnv_withStoredADC(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_prod"), "[core]\nproject = p1\n")
	writeFile(t, filepath.Join(dir, "active_config"), "prod")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "prod.json"), `{"type":"authorized_user"}`)

	stdout, stderr, err := run(t, "env", "prod")
	if err != nil {
		t.Fatalf("env prod: %v (stderr: %s)", err, stderr)
	}
	wantExport := "export CLOUDSDK_ACTIVE_CONFIG_NAME='prod'\n"
	if !strings.Contains(stdout, wantExport) {
		t.Errorf("stdout missing %q:\n%s", wantExport, stdout)
	}
	wantADC := "export GOOGLE_APPLICATION_CREDENTIALS='" + filepath.Join(dir, "gcloud-ctx", "adc", "prod.json") + "'\n"
	if !strings.Contains(stdout, wantADC) {
		t.Errorf("stdout missing %q:\n%s", wantADC, stdout)
	}
	if !strings.Contains(stdout, `# eval "$(gcloud-ctx env prod)"`) {
		t.Errorf("stdout missing eval hint comment:\n%s", stdout)
	}
	if strings.Contains(stdout, "unset ") {
		t.Errorf("stdout should not contain unset lines when a snapshot exists:\n%s", stdout)
	}
}

func TestEnv_defaultsToCurrent(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_dev"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "dev")

	stdout, _, err := run(t, "env")
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	if !strings.Contains(stdout, "export CLOUDSDK_ACTIVE_CONFIG_NAME='dev'\n") {
		t.Errorf("expected current context dev in exports:\n%s", stdout)
	}
}

func TestEnv_noStoredADC_unsetsAndHints(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_dev"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "dev")

	stdout, stderr, err := run(t, "env", "dev")
	if err != nil {
		t.Fatalf("env dev: %v", err)
	}
	if !strings.Contains(stdout, "unset GOOGLE_APPLICATION_CREDENTIALS\n") {
		t.Errorf("expected unset line for missing snapshot:\n%s", stdout)
	}
	if strings.Contains(stdout, "export GOOGLE_APPLICATION_CREDENTIALS") {
		t.Errorf("must not export ADC path without a snapshot:\n%s", stdout)
	}
	if !strings.Contains(stderr, `no stored ADC for "dev"`) {
		t.Errorf("expected stderr hint about missing snapshot, got:\n%s", stderr)
	}
}

func TestEnv_none(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "active_config"), "NONE")

	stdout, stderr, err := run(t, "env", "NONE")
	if err != nil {
		t.Fatalf("env NONE: %v", err)
	}
	if !strings.Contains(stdout, "export CLOUDSDK_ACTIVE_CONFIG_NAME='NONE'\n") {
		t.Errorf("expected NONE export:\n%s", stdout)
	}
	if !strings.Contains(stdout, "unset GOOGLE_APPLICATION_CREDENTIALS\n") {
		t.Errorf("NONE never has a snapshot; expected unset line:\n%s", stdout)
	}
	if strings.Contains(stderr, "no stored ADC") {
		t.Errorf("NONE should not produce the adc-save hint, got:\n%s", stderr)
	}
}

func TestEnv_missingContext(t *testing.T) {
	testEnv(t)
	_, _, err := run(t, "env", "nope")
	if err == nil || !strings.Contains(err.Error(), `configuration "nope" does not exist`) {
		t.Fatalf("expected does-not-exist error, got %v", err)
	}
}

func TestEnv_invalidName(t *testing.T) {
	testEnv(t)
	_, _, err := run(t, "env", "../../etc")
	if err == nil {
		t.Fatal("expected validation error for traversal-shaped name")
	}
}

func TestEnv_unset(t *testing.T) {
	testEnv(t)
	stdout, _, err := run(t, "env", "--unset")
	if err != nil {
		t.Fatalf("env --unset: %v", err)
	}
	for _, want := range []string{
		"unset CLOUDSDK_ACTIVE_CONFIG_NAME\n",
		"unset GOOGLE_APPLICATION_CREDENTIALS\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "export ") {
		t.Errorf("--unset must not export anything:\n%s", stdout)
	}
}

func TestEnv_unsetRejectsName(t *testing.T) {
	testEnv(t)
	_, _, err := run(t, "env", "--unset", "dev")
	if err == nil || !strings.Contains(err.Error(), "--unset takes no NAME") {
		t.Fatalf("expected --unset+NAME rejection, got %v", err)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"prod":         "'prod'",
		"a b":          "'a b'",
		"it's":         `'it'\''s'`,
		"$HOME/x.json": "'$HOME/x.json'",
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}
