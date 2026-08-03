package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubADCLogin replaces the interactive gcloud login with a fake that writes
// newLive as the live ADC file, restoring the real implementation afterwards.
func stubADCLogin(t *testing.T, dir, newLive string) *[]string {
	t.Helper()
	orig := adcLoginRun
	var gotArgs []string
	adcLoginRun = func(extraArgs ...string) error {
		gotArgs = append(gotArgs, extraArgs...)
		return os.WriteFile(filepath.Join(dir, "application_default_credentials.json"), []byte(newLive), 0o600)
	}
	t.Cleanup(func() { adcLoginRun = orig })
	return &gotArgs
}

// failADCLogin replaces the login stub with one that fails without touching
// anything.
func failADCLogin(t *testing.T) {
	t.Helper()
	orig := adcLoginRun
	adcLoginRun = func(...string) error { return fmt.Errorf("login aborted") }
	t.Cleanup(func() { adcLoginRun = orig })
}

func userADC(account, token string) string {
	return fmt.Sprintf(`{"account":%q,"client_id":"cid","client_secret":"cs","refresh_token":%q,"type":"authorized_user","universe_domain":"googleapis.com"}`, account, token)
}

func impersonatedADC(sa, account, token string) string {
	return fmt.Sprintf(`{"delegates":["d1@p.iam.gserviceaccount.com"],"quota_project_id":"qp","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken","source_credentials":%s,"type":"impersonated_service_account"}`, sa, userADC(account, token))
}

func snapshotJSON(t *testing.T, dir, name string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "gcloud-ctx", "adc", name+".json"))), &m); err != nil {
		t.Fatalf("snapshot %s: %v", name, err)
	}
	return m
}

func TestRefresh_plainTarget_cascadesToDependents(t *testing.T) {
	dir := testEnv(t)
	for _, n := range []string{"work", "deploy", "personal"} {
		writeFile(t, filepath.Join(dir, "configurations", "config_"+n), "")
	}
	writeFile(t, filepath.Join(dir, "active_config"), "work")
	// work: plain snapshot (account A, old token); deploy: impersonated on A;
	// personal: account B — must be untouched.
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "work.json"), userADC("a@example.com", "OLD"))
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "deploy.json"), impersonatedADC("sa@p.iam.gserviceaccount.com", "a@example.com", "OLD"))
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "personal.json"), userADC("b@example.com", "B-TOK"))
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), userADC("a@example.com", "OLD"))

	stubADCLogin(t, dir, userADC("a@example.com", "NEW"))
	_, stderr, err := run(t, "refresh")
	if err != nil {
		t.Fatalf("refresh: %v (stderr: %s)", err, stderr)
	}

	if got := snapshotJSON(t, dir, "work")["refresh_token"]; got != "NEW" {
		t.Errorf("work snapshot token = %v, want NEW", got)
	}
	dep := snapshotJSON(t, dir, "deploy")
	src := dep["source_credentials"].(map[string]any)
	if src["refresh_token"] != "NEW" {
		t.Errorf("deploy source token = %v, want NEW", src["refresh_token"])
	}
	if !strings.Contains(dep["service_account_impersonation_url"].(string), "sa@p.iam.gserviceaccount.com") {
		t.Errorf("deploy impersonation target lost: %v", dep["service_account_impersonation_url"])
	}
	if dep["quota_project_id"] != "qp" {
		t.Errorf("deploy quota project lost: %v", dep["quota_project_id"])
	}
	if got := snapshotJSON(t, dir, "personal")["refresh_token"]; got != "B-TOK" {
		t.Errorf("personal (other account) must be untouched, token = %v", got)
	}
	if !strings.Contains(stderr, `Refreshed ADC for context "work"`) {
		t.Errorf("missing refresh message:\n%s", stderr)
	}
	if !strings.Contains(stderr, "Rebuilt 1 dependent snapshot(s): deploy") {
		t.Errorf("missing cascade message:\n%s", stderr)
	}
	// Live ADC must match the active context's (work) refreshed snapshot.
	var live map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "application_default_credentials.json"))), &live); err != nil {
		t.Fatal(err)
	}
	if live["refresh_token"] != "NEW" || live["type"] != "authorized_user" {
		t.Errorf("live ADC not refreshed: %v", live)
	}
}

func TestRefresh_impersonatedActiveTarget_reinstallsImpersonatedLive(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_deploy"), "[auth]\nimpersonate_service_account = sa@p.iam.gserviceaccount.com\n")
	writeFile(t, filepath.Join(dir, "active_config"), "deploy")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "deploy.json"), impersonatedADC("sa@p.iam.gserviceaccount.com", "a@example.com", "OLD"))
	writeFile(t, filepath.Join(dir, "application_default_credentials.json"), impersonatedADC("sa@p.iam.gserviceaccount.com", "a@example.com", "OLD"))

	// gcloud login writes a PLAIN user credential as the live ADC.
	stubADCLogin(t, dir, userADC("a@example.com", "NEW"))
	_, stderr, err := run(t, "refresh", "deploy")
	if err != nil {
		t.Fatalf("refresh deploy: %v (stderr: %s)", err, stderr)
	}

	snap := snapshotJSON(t, dir, "deploy")
	if snap["type"] != "impersonated_service_account" {
		t.Fatalf("snapshot must stay impersonated, got %v", snap["type"])
	}
	if snap["source_credentials"].(map[string]any)["refresh_token"] != "NEW" {
		t.Errorf("snapshot source not refreshed")
	}
	// The live ADC must be the rebuilt impersonated credential again, not
	// the plain user credential the login flow left behind.
	var live map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "application_default_credentials.json"))), &live); err != nil {
		t.Fatal(err)
	}
	if live["type"] != "impersonated_service_account" {
		t.Errorf("live ADC type = %v, want impersonated_service_account", live["type"])
	}
	if live["source_credentials"].(map[string]any)["refresh_token"] != "NEW" {
		t.Errorf("live ADC source not refreshed")
	}
}

func TestRefresh_warnsOnAccountChange(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "work")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "work.json"), userADC("a@example.com", "OLD"))

	stubADCLogin(t, dir, userADC("other@example.com", "NEW"))
	_, stderr, err := run(t, "refresh")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !strings.Contains(stderr, `logged in as "other@example.com"`) {
		t.Errorf("expected account-change warning, got:\n%s", stderr)
	}
}

func TestRefresh_noSnapshot_createsOne(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "work")

	stubADCLogin(t, dir, userADC("a@example.com", "NEW"))
	if _, _, err := run(t, "refresh"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if got := snapshotJSON(t, dir, "work")["refresh_token"]; got != "NEW" {
		t.Errorf("expected fresh snapshot for work, token = %v", got)
	}
}

func TestRefresh_loginFailure_changesNothing(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "work")
	writeFile(t, filepath.Join(dir, "gcloud-ctx", "adc", "work.json"), userADC("a@example.com", "OLD"))

	failADCLogin(t)
	_, _, err := run(t, "refresh")
	if err == nil || !strings.Contains(err.Error(), "login aborted") {
		t.Fatalf("expected login failure, got %v", err)
	}
	if got := snapshotJSON(t, dir, "work")["refresh_token"]; got != "OLD" {
		t.Errorf("snapshot must be untouched on login failure, token = %v", got)
	}
}

func TestRefresh_forwardsBrowserFlags(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_work"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "work")

	got := stubADCLogin(t, dir, userADC("a@example.com", "NEW"))
	_, stderr, err := run(t, "refresh", "--no-launch-browser")
	if err != nil {
		t.Fatalf("refresh --no-launch-browser: %v", err)
	}
	if len(*got) != 1 || (*got)[0] != "--no-launch-browser" {
		t.Errorf("forwarded args = %v, want [--no-launch-browser]", *got)
	}
	if !strings.Contains(stderr, "gcloud auth application-default login --no-launch-browser") {
		t.Errorf("status line should echo the forwarded flag:\n%s", stderr)
	}

	got2 := stubADCLogin(t, dir, userADC("a@example.com", "NEW2"))
	if _, _, err := run(t, "refresh", "--no-browser"); err != nil {
		t.Fatalf("refresh --no-browser: %v", err)
	}
	if len(*got2) != 1 || (*got2)[0] != "--no-browser" {
		t.Errorf("forwarded args = %v, want [--no-browser]", *got2)
	}

	if _, _, err := run(t, "refresh", "--no-browser", "--no-launch-browser"); err == nil {
		t.Fatal("expected mutually-exclusive flag error")
	}
}

func TestRefresh_refusesNONEAndMissing(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "active_config"), "NONE")
	if _, _, err := run(t, "refresh"); err == nil {
		t.Fatal("expected refusal for NONE")
	}
	if _, _, err := run(t, "refresh", "nope"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected does-not-exist, got %v", err)
	}
}
