package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

const baseADC = `{"client_id":"cid","client_secret":"secret","refresh_token":"rt","type":"authorized_user"}`

// wantSynthesized is the golden impersonated ADC document gcloud-ctx must
// produce for baseADC impersonating sa@proj.iam.gserviceaccount.com with
// --delegates d1@proj.iam.gserviceaccount.com,d2@proj.iam.gserviceaccount.com
// --quota-project qp: sorted keys, 2-space indent, no trailing newline
// (matches adc.Marshal / gcloud's json.dumps(sort_keys=True, indent=2)).
const wantSynthesized = `{
  "delegates": [
    "d1@proj.iam.gserviceaccount.com",
    "d2@proj.iam.gserviceaccount.com"
  ],
  "quota_project_id": "qp",
  "service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@proj.iam.gserviceaccount.com:generateAccessToken",
  "source_credentials": {
    "client_id": "cid",
    "client_secret": "secret",
    "refresh_token": "rt",
    "type": "authorized_user"
  },
  "type": "impersonated_service_account"
}`

func TestImpersonate_endToEnd(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "[core]\naccount = a@example.com\n")
	writeFile(t, filepath.Join(dir, "active_config"), "work")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), baseADC)

	_, stderr, err := run(t, "impersonate", "sa@proj.iam.gserviceaccount.com",
		"--delegates", "d1@proj.iam.gserviceaccount.com,d2@proj.iam.gserviceaccount.com", "--quota-project", "qp")
	if err != nil {
		t.Fatalf("impersonate error = %v, stderr = %s", err, stderr)
	}

	// The INI property must be written as gcloud's native comma chain:
	// delegates first, target last.
	ini := readFile(t, filepath.Join(dir, "configurations", "config_work"))
	if !strings.Contains(ini, "impersonate_service_account = d1@proj.iam.gserviceaccount.com,d2@proj.iam.gserviceaccount.com,sa@proj.iam.gserviceaccount.com") {
		t.Errorf("config_work = %q, want auth.impersonate_service_account set to the delegate,...,target chain", ini)
	}

	// The stored ADC must match the golden synthesized document exactly.
	stored := readFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "work.json"))
	if stored != wantSynthesized {
		t.Errorf("stored ADC =\n%s\nwant:\n%s", stored, wantSynthesized)
	}

	// work is the active context, so the live ADC must also be installed.
	live := readFile(t, filepath.Join(dir, "application_default_credentials.json"))
	if live != wantSynthesized {
		t.Errorf("live ADC =\n%s\nwant:\n%s", live, wantSynthesized)
	}

	// show must split the chain and render target/delegates distinctly.
	stdout, _, err := run(t, "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Impersonation: sa@proj.iam.gserviceaccount.com") {
		t.Errorf("show stdout = %q, want target-only Impersonation line", stdout)
	}
	if !strings.Contains(stdout, "Delegates:     d1@proj.iam.gserviceaccount.com, d2@proj.iam.gserviceaccount.com") {
		t.Errorf("show stdout = %q, want a Delegates line listing both delegates in order", stdout)
	}

	// -l table's IMPERSONATION column shows the resolved target, not the raw chain.
	stdoutLong, _, err := run(t, "-l")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdoutLong, "sa@proj.iam.gserviceaccount.com") {
		t.Errorf("-l stdout = %q, want the target SA in the table", stdoutLong)
	}
	if strings.Contains(stdoutLong, "d1@proj.iam.gserviceaccount.com,d2@proj.iam.gserviceaccount.com,sa@proj.iam.gserviceaccount.com") {
		t.Errorf("-l stdout = %q, want the raw chain not dumped verbatim into the table", stdoutLong)
	}
}

func TestImpersonate_notActive_doesNotTouchLiveADC(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "")
	writeFile(t, filepath.Join(dir, "configurations", "config_other"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "other")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "work.json"), baseADC)
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), "unrelated live ADC")

	if _, _, err := run(t, "impersonate", "sa@proj.iam.gserviceaccount.com", "--context", "work"); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(dir, "application_default_credentials.json")); got != "unrelated live ADC" {
		t.Errorf("live ADC changed even though 'work' isn't active: %q", got)
	}
}

func TestImpersonate_contextOther_usesLiveADC_printsNotice(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "[core]\naccount = current@example.com\n")
	writeFile(t, filepath.Join(dir, "configurations", "config_other"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), baseADC)

	_, stderr, err := run(t, "impersonate", "sa@proj.iam.gserviceaccount.com", "--context", "other")
	if err != nil {
		t.Fatalf("impersonate error = %v, stderr = %s", err, stderr)
	}
	if !strings.Contains(stderr, `using current live ADC (account current@example.com) as base credentials for context "other"`) {
		t.Errorf("stderr = %q, want the live-ADC-as-base notice naming the account and target context", stderr)
	}
}

func TestImpersonate_invalidServiceAccount(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "work")

	tests := []string{
		"not-a-service-account",
		"sa@proj.iam.gserviceaccount.com/../evil",
		"sa@proj.iam.gserviceaccount.com:8080",
		"sa@proj.iam.gserviceaccount.com?x=1",
		"sa@proj.iam.gserviceaccount.com#frag",
		"sa with space@proj.iam.gserviceaccount.com",
		"sa@proj.iam.gserviceaccount.com\nEXTRA",
	}
	for _, sa := range tests {
		if _, _, err := run(t, "impersonate", sa); err == nil {
			t.Errorf("impersonate %q: want error, got nil", sa)
		}
	}
}

func TestImpersonate_invalidDelegate(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "work")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), baseADC)

	if _, _, err := run(t, "impersonate", "sa@proj.iam.gserviceaccount.com", "--delegates", "not-valid"); err == nil {
		t.Fatal("impersonate with an invalid delegate: want error, got nil")
	}
	// Nothing should have been written since validation happens before any writes.
	if fileExists(t, filepath.Join(dir, "gcloud-ctx", "adc", "work.json")) {
		t.Error("ADC should not have been stored when a delegate fails validation")
	}
}

func TestImpersonate_refusesNone(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "active_config"), "NONE")

	if _, _, err := run(t, "impersonate", "sa@proj.iam.gserviceaccount.com"); err == nil {
		t.Fatal("impersonate targeting NONE: want error, got nil")
	}
}

func TestImpersonate_noBaseADC(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "work")

	if _, _, err := run(t, "impersonate", "sa@proj.iam.gserviceaccount.com"); err == nil {
		t.Fatal("impersonate with no live or stored ADC: want error, got nil")
	}
}

func TestImpersonate_propertyWriteFails_reportsADCAlreadyUpdated(t *testing.T) {
	dir := testEnv(t)
	// A directory (not a file) named config_work makes the INI write fail
	// (it's a load-whole-file/write-whole-file mutation), while ADC store
	// writes (under gcloud-ctx/adc/, an unrelated path) still succeed.
	if err := writeDir(filepath.Join(dir, "configurations", "config_work")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "active_config"), "work")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), baseADC)

	_, _, err := run(t, "impersonate", "sa@proj.iam.gserviceaccount.com")
	if err == nil {
		t.Fatal("impersonate with unwritable INI file: want error, got nil")
	}
	if !strings.Contains(err.Error(), "ADC updated") || !strings.Contains(err.Error(), "gcloud property was not set") {
		t.Errorf("error = %v, want it to explicitly state ADC was updated but the property was not", err)
	}
	if !fileExists(t, filepath.Join(dir, "gcloud-ctx", "adc", "work.json")) {
		t.Error("ADC should have been stored before the property write was attempted")
	}
}

func TestImpersonate_clear(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "[auth]\nimpersonate_service_account = sa@proj.iam.gserviceaccount.com\n")
	writeFile(t, filepath.Join(dir, "active_config"), "work")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "work.json"), wantSynthesized)
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), wantSynthesized)

	if _, _, err := run(t, "impersonate", "--clear"); err != nil {
		t.Fatal(err)
	}

	ini := readFile(t, filepath.Join(dir, "configurations", "config_work"))
	if strings.Contains(ini, "impersonate_service_account") {
		t.Errorf("config_work = %q, want impersonate_service_account removed", ini)
	}

	stored := readFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "work.json"))
	if strings.Contains(stored, "impersonated_service_account") {
		t.Errorf("stored ADC still impersonated: %s", stored)
	}
	if !strings.Contains(stored, `"type": "authorized_user"`) {
		t.Errorf("stored ADC = %q, want unwrapped authorized_user", stored)
	}

	live := readFile(t, filepath.Join(dir, "application_default_credentials.json"))
	if live != stored {
		t.Errorf("live ADC (%q) should match the unwrapped stored ADC (%q)", live, stored)
	}
}

// TestImpersonate_clear_liveOnlyUnwrap covers the new --clear behavior: when
// there is no stored ADC snapshot at all, but the *live* ADC of the
// raw-active context is impersonated, --clear must still unwrap and install
// the live file directly.
func TestImpersonate_clear_liveOnlyUnwrap(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "[auth]\nimpersonate_service_account = sa@proj.iam.gserviceaccount.com\n")
	writeFile(t, filepath.Join(dir, "active_config"), "work")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), wantSynthesized)
	// Deliberately no gcloud-ctx/adc/work.json stored snapshot.

	if _, _, err := run(t, "impersonate", "--clear"); err != nil {
		t.Fatal(err)
	}

	live := readFile(t, filepath.Join(dir, "application_default_credentials.json"))
	if strings.Contains(live, "impersonated_service_account") {
		t.Errorf("live ADC still impersonated after --clear with no stored snapshot: %s", live)
	}
	if !strings.Contains(live, `"type": "authorized_user"`) {
		t.Errorf("live ADC = %q, want unwrapped authorized_user", live)
	}

	ini := readFile(t, filepath.Join(dir, "configurations", "config_work"))
	if strings.Contains(ini, "impersonate_service_account") {
		t.Errorf("config_work = %q, want impersonate_service_account removed", ini)
	}
}

// TestImpersonate_clear_liveUnwrappableWarns covers the case where the live
// ADC of the raw-active context claims to be impersonated but can't actually
// be unwrapped (malformed source_credentials): --clear must still succeed
// (the INI property still gets cleared) and warn on stderr instead of
// failing outright.
func TestImpersonate_clear_liveUnwrappableWarns(t *testing.T) {
	dir := testEnv(t)
	badLive := `{"type":"impersonated_service_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@proj.iam.gserviceaccount.com:generateAccessToken"}`
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "[auth]\nimpersonate_service_account = sa@proj.iam.gserviceaccount.com\n")
	writeFile(t, filepath.Join(dir, "active_config"), "work")
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), badLive)

	_, stderr, err := run(t, "impersonate", "--clear")
	if err != nil {
		t.Fatalf("clear with unwrappable live ADC: want success (with a warning), got error = %v", err)
	}
	if !strings.Contains(stderr, "warning") || !strings.Contains(stderr, "still impersonates") {
		t.Errorf("stderr = %q, want a warning that the live ADC still impersonates", stderr)
	}
	ini := readFile(t, filepath.Join(dir, "configurations", "config_work"))
	if strings.Contains(ini, "impersonate_service_account") {
		t.Errorf("config_work = %q, want impersonate_service_account removed despite the live-ADC warning", ini)
	}
}
