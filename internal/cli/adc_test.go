package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestADCSave(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), `{"type":"authorized_user"}`)

	if _, stderr, err := run(t, "adc", "save"); err != nil {
		t.Fatalf("adc save error = %v, stderr = %s", err, stderr)
	}

	got := readFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "default.json"))
	if got != `{"type":"authorized_user"}` {
		t.Errorf("stored ADC = %q, want the live ADC contents copied verbatim", got)
	}
}

func TestADCSave_explicitName(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_other"), "")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), `{"type":"authorized_user"}`)

	if _, _, err := run(t, "adc", "save", "other"); err != nil {
		t.Fatal(err)
	}
	if !fileExists(t, filepath.Join(dir, "gcloud-ctx", "adc", "other.json")) {
		t.Error("ADC not saved for explicitly named context")
	}
}

func TestADCSave_noLiveADC(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	if _, _, err := run(t, "adc", "save"); err == nil {
		t.Fatal("adc save with no live ADC file: want error, got nil")
	}
}

func TestADCSave_unknownContext(t *testing.T) {
	testEnv(t)
	if _, _, err := run(t, "adc", "save", "nope"); err == nil {
		t.Fatal("adc save for a nonexistent context: want error, got nil")
	}
}

// TestADCSave_refusesNone covers item 5: "adc save" must refuse to bind an
// ADC to the virtual NONE configuration, whether NONE is named explicitly or
// resolved as the current context.
func TestADCSave_refusesNone(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), `{"type":"authorized_user"}`)

	if _, _, err := run(t, "adc", "save", "NONE"); err == nil {
		t.Fatal("adc save NONE: want error, got nil")
	} else if !strings.Contains(err.Error(), "NONE") {
		t.Errorf("error = %v, want it to mention NONE", err)
	}

	writeFile(t, filepath.Join(dir, "active_config"), "NONE")
	if _, _, err := run(t, "adc", "save"); err == nil {
		t.Fatal("adc save with NONE as the current context: want error, got nil")
	}
}

// TestADCSave_unparseableLiveADC covers item 5: the live ADC must parse and
// type-check before being stored.
func TestADCSave_unparseableLiveADC(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), "not valid json")

	if _, _, err := run(t, "adc", "save"); err == nil {
		t.Fatal("adc save with unparseable live ADC: want error, got nil")
	}
	if fileExists(t, filepath.Join(dir, "gcloud-ctx", "adc", "default.json")) {
		t.Error("ADC should not have been stored when the live ADC is unparseable")
	}
}

// TestADCSave_unrecognizedType covers item 5's type check: valid JSON with
// no recognized "type" field must still be refused.
func TestADCSave_unrecognizedType(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), `{"type":"something_unknown"}`)

	if _, _, err := run(t, "adc", "save"); err == nil {
		t.Fatal("adc save with unrecognized credential type: want error, got nil")
	}
	if fileExists(t, filepath.Join(dir, "gcloud-ctx", "adc", "default.json")) {
		t.Error("ADC should not have been stored when its type is unrecognized")
	}
}
