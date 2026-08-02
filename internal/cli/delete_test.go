package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDelete_basic(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_stale"), "")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "stale.json"), `{"type":"authorized_user"}`)
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "previous"), "stale")

	if _, stderr, err := run(t, "-d", "stale"); err != nil {
		t.Fatalf("delete error = %v, stderr = %s", err, stderr)
	}

	if fileExists(t, filepath.Join(dir, "configurations", "config_stale")) {
		t.Error("config file not removed")
	}
	if fileExists(t, filepath.Join(dir, "gcloud-ctx", "adc", "stale.json")) {
		t.Error("stored ADC not removed")
	}
	if fileExists(t, filepath.Join(dir, "gcloud-ctx", "previous")) {
		t.Error("previous record not cleared after deleting the context it pointed at")
	}
}

func TestDelete_refusesActive(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	if _, _, err := run(t, "-d", "default"); err == nil {
		t.Fatal("delete of active context: want error, got nil")
	}
	if !fileExists(t, filepath.Join(dir, "configurations", "config_default")) {
		t.Error("active context should not have been deleted")
	}
}

func TestDelete_refusesNone(t *testing.T) {
	testEnv(t)
	if _, _, err := run(t, "-d", "NONE"); err == nil {
		t.Fatal("delete of NONE: want error, got nil")
	}
}

func TestDelete_multiplePartialFailure(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_a"), "")
	writeFile(t, filepath.Join(dir, "configurations", "config_b"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "b")

	_, stderr, err := run(t, "-d", "a", "b")
	if err == nil {
		t.Fatal("delete with one failing name: want error, got nil")
	}
	if fileExists(t, filepath.Join(dir, "configurations", "config_a")) {
		t.Error("'a' should have been deleted despite 'b' failing")
	}
	if !fileExists(t, filepath.Join(dir, "configurations", "config_b")) {
		t.Error("'b' (active) should not have been deleted")
	}
	if !strings.Contains(stderr, "error:") {
		t.Errorf("stderr = %q, want an inline error: line for the failed name", stderr)
	}
}
