package gcloud

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newTestStore returns a Store rooted at a fresh t.TempDir(), with the
// configurations subdirectory already created.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, configurationsSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	return NewStore(dir)
}

func writeConfig(t *testing.T, s *Store, name, content string) {
	t.Helper()
	path, err := s.ConfigPath(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// configPath is a test helper that fails the test on an unexpected
// ConfigPath error, for callers that already know name is valid.
func configPath(t *testing.T, s *Store, name string) string {
	t.Helper()
	path, err := s.ConfigPath(name)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// badNames are the invalid-name attack surface every path-building method
// must reject: path traversal, an embedded traversal segment, empty, ".",
// and the reserved NONE pseudo-configuration.
var badNames = []string{"../../../x", "a/../b", "", ".", "NONE"}

func TestStore_List(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "default", "[core]\naccount = a@example.com\nproject = proj-a\n")
	writeConfig(t, s, "staging", "[core]\naccount = b@example.com\nproject = proj-b\n\n[compute]\nregion = us-east1\n")
	// A malformed filename must be skipped, not error the whole List. Written
	// directly (not via writeConfig/ConfigPath, which now validate the name)
	// to simulate a stray file with a name gcloud-ctx itself would never
	// produce.
	if err := os.WriteFile(filepath.Join(s.configurationsDir(), configFilePrefix+"Not_Valid"), []byte("[core]\naccount = bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.configurationsDir(), "not-a-config"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() returned %d configs, want 2: %+v", len(got), got)
	}
	if got[0].Name != "default" || got[1].Name != "staging" {
		t.Fatalf("List() names = [%s, %s], want [default, staging] (sorted)", got[0].Name, got[1].Name)
	}
	if got[0].Account != "a@example.com" || got[0].Project != "proj-a" {
		t.Errorf("List()[0] properties = %+v", got[0])
	}
	if got[1].Region != "us-east1" {
		t.Errorf("List()[1].Region = %q, want us-east1", got[1].Region)
	}
	if got[0].ParseErr != nil || got[1].ParseErr != nil {
		t.Errorf("ParseErr on well-formed configs = %v / %v, want nil", got[0].ParseErr, got[1].ParseErr)
	}
}

func TestStore_List_missingDir(t *testing.T) {
	s := NewStore(t.TempDir()) // no configurations dir created
	got, err := s.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List() = %+v, want empty", got)
	}
}

// TestStore_List_parseErrorDoesNotAbort is the robustness test required by
// DESIGN.md: a config file that fails to parse must still show up in the
// listing (with a ParseErr, and empty properties) rather than aborting the
// whole List() or silently vanishing.
func TestStore_List_parseErrorDoesNotAbort(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "good", "[core]\naccount = a@example.com\n")
	// Garbage that fails to parse as INI at all: a bare value line with no
	// section and no key/value delimiter.
	writeConfig(t, s, "garbage", "this is not valid ini \x00\x01 content")

	got, err := s.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() returned %d configs, want 2 (parse failure must still be listed): %+v", len(got), got)
	}
	if got[0].Name != "garbage" || got[1].Name != "good" {
		t.Fatalf("List() names = [%s, %s], want [garbage, good] (sorted)", got[0].Name, got[1].Name)
	}
	if got[0].ParseErr == nil {
		t.Error("garbage config ParseErr = nil, want non-nil")
	}
	if got[0].Properties != (Properties{}) {
		t.Errorf("garbage config Properties = %+v, want zero value", got[0].Properties)
	}
	if got[1].ParseErr != nil {
		t.Errorf("good config ParseErr = %v, want nil", got[1].ParseErr)
	}
}

func TestStore_ActiveName(t *testing.T) {
	s := newTestStore(t)

	name, err := s.ActiveName()
	if err != nil {
		t.Fatalf("ActiveName() error = %v", err)
	}
	if name != "" {
		t.Fatalf("ActiveName() = %q, want empty when active_config is missing", name)
	}

	if err := os.WriteFile(filepath.Join(s.Dir, activeConfigFile), []byte("staging"), 0o644); err != nil {
		t.Fatal(err)
	}
	name, err = s.ActiveName()
	if err != nil {
		t.Fatalf("ActiveName() error = %v", err)
	}
	if name != "staging" {
		t.Fatalf("ActiveName() = %q, want staging", name)
	}
}

func TestStore_EffectiveActiveName(t *testing.T) {
	s := newTestStore(t)
	if err := os.WriteFile(filepath.Join(s.Dir, activeConfigFile), []byte("staging"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.EffectiveActiveName()
	if err != nil {
		t.Fatalf("EffectiveActiveName() error = %v", err)
	}
	if got != "staging" {
		t.Fatalf("EffectiveActiveName() = %q, want staging (from file)", got)
	}

	t.Setenv(ActiveConfigEnvVar, "prod")
	got, err = s.EffectiveActiveName()
	if err != nil {
		t.Fatalf("EffectiveActiveName() error = %v", err)
	}
	if got != "prod" {
		t.Fatalf("EffectiveActiveName() = %q, want prod (env var shadows file, even with no backing file)", got)
	}
}

func TestStore_Activate(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "staging", "")

	if err := s.Activate("staging"); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	got, err := s.ActiveName()
	if err != nil {
		t.Fatal(err)
	}
	if got != "staging" {
		t.Fatalf("ActiveName() after Activate = %q, want staging", got)
	}

	// Exact bytes, no trailing newline.
	raw, err := os.ReadFile(filepath.Join(s.Dir, activeConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "staging" {
		t.Fatalf("active_config raw bytes = %q, want exactly %q (no trailing newline)", raw, "staging")
	}

	sentinel := filepath.Join(s.Dir, configSentinelFile)
	info, err := os.Stat(sentinel)
	if err != nil {
		t.Fatalf("config_sentinel not created: %v", err)
	}
	firstMtime := info.ModTime()

	// Touch again and confirm the sentinel's mtime bumps forward.
	time.Sleep(10 * time.Millisecond)
	if err := s.Activate("staging"); err != nil {
		t.Fatalf("Activate() (again) error = %v", err)
	}
	info, err = os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().After(firstMtime) {
		t.Errorf("config_sentinel mtime did not advance: before=%v after=%v", firstMtime, info.ModTime())
	}
}

func TestStore_Activate_none(t *testing.T) {
	s := newTestStore(t)
	if err := s.Activate(NoneConfig); err != nil {
		t.Fatalf("Activate(NONE) error = %v", err)
	}
	got, err := s.ActiveName()
	if err != nil {
		t.Fatal(err)
	}
	if got != NoneConfig {
		t.Fatalf("ActiveName() = %q, want NONE", got)
	}
}

func TestStore_Activate_nonexistent(t *testing.T) {
	s := newTestStore(t)
	if err := s.Activate("ghost"); err == nil {
		t.Fatal("Activate(nonexistent) error = nil, want error")
	}
}

func TestStore_Create(t *testing.T) {
	s := newTestStore(t)
	props := Properties{Account: "a@example.com", Project: "proj", Region: "us-central1", ImpersonateServiceAccount: "sa@x.iam.gserviceaccount.com"}
	if err := s.Create("newctx", props); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := s.GetProperties("newctx")
	if err != nil {
		t.Fatalf("GetProperties() error = %v", err)
	}
	if got != props {
		t.Fatalf("GetProperties() = %+v, want %+v", got, props)
	}
}

func TestStore_Create_emptyProperties(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create("newctx", Properties{}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	data, err := os.ReadFile(configPath(t, s, "newctx"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Errorf("Create() with empty properties wrote %d bytes, want a 0-byte file: %q", len(data), data)
	}
}

func TestStore_Create_rejectsExistingAndReserved(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "existing", "")

	if err := s.Create("existing", Properties{}); err == nil {
		t.Error("Create(existing) error = nil, want error")
	}
	if err := s.Create(NoneConfig, Properties{}); err == nil {
		t.Error("Create(NONE) error = nil, want error")
	}
	if err := s.Create("Bad Name", Properties{}); err == nil {
		t.Error("Create(invalid name) error = nil, want error")
	}
}

func TestStore_Create_touchesSentinel(t *testing.T) {
	s := newTestStore(t)
	sentinel := filepath.Join(s.Dir, configSentinelFile)
	if err := s.Create("first", Properties{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(sentinel)
	if err != nil {
		t.Fatalf("config_sentinel not created by Create(): %v", err)
	}
	firstMtime := info.ModTime()

	time.Sleep(10 * time.Millisecond)
	if err := s.Create("second", Properties{}); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().After(firstMtime) {
		t.Errorf("config_sentinel mtime did not advance after second Create(): before=%v after=%v", firstMtime, info.ModTime())
	}
}

func TestStore_Create_rejectsInvalidPropertyValues(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create("ctx", Properties{Account: "a@example.com\nauth.impersonate_service_account = evil@x.iam.gserviceaccount.com"}); err == nil {
		t.Error("Create() with a newline-injecting property value error = nil, want error")
	}
	// Create must not have left a half-written, corrupt config behind.
	if exists, _ := s.Exists("ctx"); exists {
		t.Error("Create() with invalid property value left a config file behind, want none")
	}
}

func TestStore_Delete(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "gone", "")

	if err := s.Delete("gone"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(configPath(t, s, "gone")); !os.IsNotExist(err) {
		t.Error("config file still exists after Delete")
	}
}

func TestStore_Delete_touchesSentinel(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "gone", "")
	sentinel := filepath.Join(s.Dir, configSentinelFile)
	if err := touch(sentinel); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)
	if err := s.Delete("gone"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().After(before.ModTime()) {
		t.Errorf("config_sentinel mtime did not advance after Delete(): before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

func TestStore_Delete_refusesActiveAndNone(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "cur", "")
	if err := s.Activate("cur"); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete("cur"); err == nil {
		t.Error("Delete(active) error = nil, want error")
	}
	if err := s.Delete(NoneConfig); err == nil {
		t.Error("Delete(NONE) error = nil, want error")
	}
	if err := s.Delete("ghost"); err == nil {
		t.Error("Delete(nonexistent) error = nil, want error")
	}
}

func TestStore_Delete_refusesViaEnvVarActive(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "other", "")
	writeConfig(t, s, "shadowed", "")
	if err := s.Activate("other"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ActiveConfigEnvVar, "shadowed")

	if err := s.Delete("shadowed"); err == nil {
		t.Error("Delete() of env-var-active config error = nil, want error")
	}
	// The raw active one must still be deletable-blocked check independent: "other" is not env-active but is raw-active.
	if err := s.Delete("other"); err == nil {
		t.Error("Delete() of raw-active config error = nil, want error")
	}
}

func TestStore_Delete_rejectsInvalidNames(t *testing.T) {
	s := newTestStore(t)
	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			if err := s.Delete(name); err == nil {
				t.Errorf("Delete(%q) error = nil, want error", name)
			}
		})
	}
	// None of those attempts may have touched anything outside the
	// configurations dir (defense-in-depth against path escape).
	entries, err := os.ReadDir(filepath.Dir(s.configurationsDir()))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != configurationsSubdir {
			t.Errorf("unexpected entry %q created outside configurations dir", e.Name())
		}
	}
}

func TestStore_Rename(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "old", "[core]\naccount = a@example.com\n")

	if _, err := s.Rename("old", "new"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if _, err := os.Stat(configPath(t, s, "old")); !os.IsNotExist(err) {
		t.Error("old config file still exists after rename")
	}
	props, err := s.GetProperties("new")
	if err != nil {
		t.Fatalf("GetProperties(new) error = %v", err)
	}
	if props.Account != "a@example.com" {
		t.Errorf("renamed config account = %q, want a@example.com", props.Account)
	}
}

func TestStore_Rename_touchesSentinel(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "old", "")
	sentinel := filepath.Join(s.Dir, configSentinelFile)
	if err := touch(sentinel); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)
	if _, err := s.Rename("old", "new"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().After(before.ModTime()) {
		t.Errorf("config_sentinel mtime did not advance after Rename(): before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

// TestStore_Rename_active is the new-behavior test: renaming the active
// configuration must now succeed, and active_config must be rewritten to
// point at the new name so the switch stays coherent.
func TestStore_Rename_active(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "old", "")
	if err := s.Activate("old"); err != nil {
		t.Fatal(err)
	}

	result, err := s.Rename("old", "new")
	if err != nil {
		t.Fatalf("Rename() of active config error = %v, want nil (renaming the active config must succeed)", err)
	}
	if !result.WasActive {
		t.Error("RenameResult.WasActive = false, want true")
	}

	active, err := s.ActiveName()
	if err != nil {
		t.Fatal(err)
	}
	if active != "new" {
		t.Errorf("active_config after renaming the active config = %q, want new", active)
	}
	if exists, _ := s.Exists("old"); exists {
		t.Error("old config file still exists after rename")
	}
	if exists, _ := s.Exists("new"); !exists {
		t.Error("new config file does not exist after rename")
	}
}

// TestStore_Rename_envShadowed exercises the "still proceed, just tell the
// caller" path: CLOUDSDK_ACTIVE_CONFIG_NAME names oldName (shadowing the
// raw active_config, which points elsewhere). Rename cannot rewrite the
// calling shell's environment, so it must still succeed and report
// EnvShadowed for the CLI to warn about.
func TestStore_Rename_envShadowed(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "old", "")
	writeConfig(t, s, "other", "")
	if err := s.Activate("other"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ActiveConfigEnvVar, "old")

	result, err := s.Rename("old", "new")
	if err != nil {
		t.Fatalf("Rename() of env-shadowed config error = %v, want nil", err)
	}
	if !result.EnvShadowed {
		t.Error("RenameResult.EnvShadowed = false, want true")
	}
	if result.WasActive {
		t.Error("RenameResult.WasActive = true, want false (raw active_config was \"other\", not \"old\")")
	}
	// The raw active_config is untouched: it wasn't "old" to begin with.
	raw, err := s.ActiveName()
	if err != nil {
		t.Fatal(err)
	}
	if raw != "other" {
		t.Errorf("active_config = %q, want other (unchanged)", raw)
	}
}

func TestStore_Rename_rejectsBadTargets(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "old", "")
	writeConfig(t, s, "taken", "")

	if _, err := s.Rename("old", "taken"); err == nil {
		t.Error("Rename() to existing name error = nil, want error")
	}
	if _, err := s.Rename("ghost", "new"); err == nil {
		t.Error("Rename() of nonexistent config error = nil, want error")
	}
	if _, err := s.Rename(NoneConfig, "new"); err == nil {
		t.Error("Rename(NONE, ...) error = nil, want error")
	}
	if _, err := s.Rename("old", "Bad Name"); err == nil {
		t.Error("Rename() to invalid name error = nil, want error")
	}
}

func TestStore_Rename_rejectsInvalidOldNames(t *testing.T) {
	s := newTestStore(t)
	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Rename(name, "new"); err == nil {
				t.Errorf("Rename(%q, new) error = nil, want error", name)
			}
		})
	}
}

func TestStore_GetProperties_emptyFile(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "blank", "")

	props, err := s.GetProperties("blank")
	if err != nil {
		t.Fatalf("GetProperties() on empty file error = %v", err)
	}
	if props != (Properties{}) {
		t.Errorf("GetProperties() on empty file = %+v, want zero value", props)
	}
}

func TestStore_GetProperties_missingFile(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetProperties("ghost"); err == nil {
		t.Error("GetProperties(nonexistent) error = nil, want error")
	}
}

func TestStore_SetProperty_createsFile(t *testing.T) {
	s := newTestStore(t)
	// SetProperty on a name with no backing file yet should create one.
	if err := s.SetProperty("brandnew", "core", "project", "proj"); err != nil {
		t.Fatalf("SetProperty() error = %v", err)
	}
	props, err := s.GetProperties("brandnew")
	if err != nil {
		t.Fatal(err)
	}
	if props.Project != "proj" {
		t.Errorf("Project = %q, want proj", props.Project)
	}
}

func TestStore_SetProperty_touchesSentinel(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "ctx", "")
	sentinel := filepath.Join(s.Dir, configSentinelFile)
	if err := touch(sentinel); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)
	if err := s.SetProperty("ctx", "core", "project", "proj"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().After(before.ModTime()) {
		t.Errorf("config_sentinel mtime did not advance after SetProperty(): before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

// TestStore_SetProperty_rejectsNewlineInjection is the property-value
// validation test: a value trying to smuggle a second INI line (e.g. to
// silently set an unrelated key, or forge additional config) must be
// rejected with a clear error, and must not be written at all.
func TestStore_SetProperty_rejectsNewlineInjection(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "ctx", "[core]\naccount = a@example.com\n")

	malicious := "innocuous\n[auth]\nimpersonate_service_account = evil@x.iam.gserviceaccount.com"
	err := s.SetProperty("ctx", "core", "project", malicious)
	if err == nil {
		t.Fatal("SetProperty() with an embedded newline error = nil, want error")
	}
	if !strings.Contains(err.Error(), "control character") {
		t.Errorf("SetProperty() error = %q, want it to mention the control character", err)
	}

	props, err := s.GetProperties("ctx")
	if err != nil {
		t.Fatal(err)
	}
	if props.Project != "" {
		t.Errorf("Project = %q after rejected SetProperty, want unset", props.Project)
	}
	if props.ImpersonateServiceAccount != "" {
		t.Errorf("ImpersonateServiceAccount = %q after rejected newline-injection SetProperty, want unset (injection must not land)", props.ImpersonateServiceAccount)
	}
}

func TestStore_SetProperty_rejectsCarriageReturn(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "ctx", "")
	if err := s.SetProperty("ctx", "core", "project", "abc\rdef"); err == nil {
		t.Error("SetProperty() with an embedded CR error = nil, want error")
	}
}

func TestStore_DeleteProperty_missingIsNotError(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "ctx", "[core]\naccount = a@example.com\n")
	if err := s.DeleteProperty("ctx", "auth", "impersonate_service_account"); err != nil {
		t.Fatalf("DeleteProperty() on absent section error = %v", err)
	}
}

func TestStore_DeleteProperty_touchesSentinel(t *testing.T) {
	s := newTestStore(t)
	writeConfig(t, s, "ctx", "[auth]\nimpersonate_service_account = sa@x.iam.gserviceaccount.com\n")
	sentinel := filepath.Join(s.Dir, configSentinelFile)
	if err := touch(sentinel); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)
	if err := s.DeleteProperty("ctx", "auth", "impersonate_service_account"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().After(before.ModTime()) {
		t.Errorf("config_sentinel mtime did not advance after DeleteProperty(): before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

// TestStore_INIRoundTrip is the golden-file test required by DESIGN.md:
// a property write on an INI file with unknown sections/comments must
// preserve everything it doesn't touch, byte for byte.
func TestStore_INIRoundTrip(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "with_unknowns.ini"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("set property preserves unknown sections and comments", func(t *testing.T) {
		s := newTestStore(t)
		writeConfig(t, s, "ctx", string(fixture))

		if err := s.SetProperty("ctx", "core", "account", "new@example.com"); err != nil {
			t.Fatalf("SetProperty() error = %v", err)
		}
		assertGolden(t, configPath(t, s, "ctx"), filepath.Join("testdata", "with_unknowns_after_set_account.golden"))
	})

	t.Run("delete property drops emptied section, preserves the rest", func(t *testing.T) {
		s := newTestStore(t)
		writeConfig(t, s, "ctx", string(fixture))

		if err := s.DeleteProperty("ctx", "auth", "impersonate_service_account"); err != nil {
			t.Fatalf("DeleteProperty() error = %v", err)
		}
		assertGolden(t, configPath(t, s, "ctx"), filepath.Join("testdata", "with_unknowns_after_delete_impersonate.golden"))
	})
}

// TestStore_INIFidelity_specialCharsSurviveRoundTrip is the golden-fixture
// test required for fix #2: values containing '#' and ';' must survive a
// property write on an unrelated key verbatim, not get truncated at the
// first comment character.
func TestStore_INIFidelity_specialCharsSurviveRoundTrip(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "with_special_chars.ini"))
	if err != nil {
		t.Fatal(err)
	}
	s := newTestStore(t)
	writeConfig(t, s, "ctx", string(fixture))

	if err := s.SetProperty("ctx", "core", "project", "new-project"); err != nil {
		t.Fatalf("SetProperty() error = %v", err)
	}
	assertGolden(t, configPath(t, s, "ctx"), filepath.Join("testdata", "with_special_chars_after_set_project.golden"))

	props, err := s.GetProperties("ctx")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(props.Account, "#") {
		t.Errorf("Account = %q, want the literal '#' preserved", props.Account)
	}
}

// TestStore_INIFidelity_pythonMultilineValueDoesNotCrash is the other
// golden-fixture test required for fix #2: a Python-configparser-style
// indented continuation value must not make List/SetProperty error out.
func TestStore_INIFidelity_pythonMultilineValueDoesNotCrash(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "with_python_multiline.ini"))
	if err != nil {
		t.Fatal(err)
	}
	s := newTestStore(t)
	writeConfig(t, s, "ctx", string(fixture))

	configs, err := s.List()
	if err != nil {
		t.Fatalf("List() on a config with a Python-multiline value error = %v, want nil", err)
	}
	if len(configs) != 1 || configs[0].ParseErr != nil {
		t.Fatalf("List() = %+v, want one config with no ParseErr", configs)
	}

	if err := s.SetProperty("ctx", "core", "project", "still-works"); err != nil {
		t.Fatalf("SetProperty() on a config with a Python-multiline value error = %v, want nil", err)
	}
	props, err := s.GetProperties("ctx")
	if err != nil {
		t.Fatal(err)
	}
	if props.Project != "still-works" {
		t.Errorf("Project = %q, want still-works", props.Project)
	}
}

func assertGolden(t *testing.T, gotPath, goldenPath string) {
	t.Helper()
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("output does not match golden file %s\n--- got ---\n%s\n--- want ---\n%s", goldenPath, got, want)
	}
}

func TestStore_filePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits aren't meaningful on windows")
	}
	s := newTestStore(t)
	if err := s.Create("ctx", Properties{Account: "a@example.com"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(configPath(t, s, "ctx"))
	if err != nil {
		t.Fatal(err)
	}
	// Config files aren't credentials; just confirm they're not world-writable
	// nonsense from a mode mistake.
	if info.Mode().Perm()&0o022 != 0 {
		t.Errorf("config file mode = %v, want group/other not writable", info.Mode().Perm())
	}
}

func TestStore_ConfigPath_rejectsInvalidNames(t *testing.T) {
	s := newTestStore(t)
	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			if _, err := s.ConfigPath(name); err == nil {
				t.Errorf("ConfigPath(%q) error = nil, want error", name)
			}
		})
	}
}

func TestValidatePropertyValue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"plain", "sa@x.iam.gserviceaccount.com", false},
		{"with hash", "value # not a comment", false},
		{"with semicolon", "value ; not a comment", false},
		{"newline", "abc\ndef", true},
		{"carriage return", "abc\rdef", true},
		{"crlf", "abc\r\ndef", true},
		{"other control char", "abc\x00def", true},
		{"tab is a control char too", "abc\tdef", true},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePropertyValue(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePropertyValue(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}
