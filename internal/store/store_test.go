package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrevious_missing(t *testing.T) {
	s := New(t.TempDir())
	got, err := s.Previous()
	if err != nil {
		t.Fatalf("Previous() error = %v", err)
	}
	if got != "" {
		t.Errorf("Previous() = %q, want empty when no record exists", got)
	}
}

func TestSetPrevious_and_Previous(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SetPrevious("staging"); err != nil {
		t.Fatalf("SetPrevious() error = %v", err)
	}
	got, err := s.Previous()
	if err != nil {
		t.Fatalf("Previous() error = %v", err)
	}
	if got != "staging" {
		t.Errorf("Previous() = %q, want staging", got)
	}
}

func TestClearPrevious(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SetPrevious("staging"); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearPrevious(); err != nil {
		t.Fatalf("ClearPrevious() error = %v", err)
	}
	got, err := s.Previous()
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("Previous() after ClearPrevious = %q, want empty", got)
	}
	// Clearing again must not error.
	if err := s.ClearPrevious(); err != nil {
		t.Errorf("ClearPrevious() (twice) error = %v", err)
	}
}

func TestClearPreviousIfMatches(t *testing.T) {
	t.Run("clears on match", func(t *testing.T) {
		s := New(t.TempDir())
		if err := s.SetPrevious("staging"); err != nil {
			t.Fatal(err)
		}
		if err := s.ClearPreviousIfMatches("staging"); err != nil {
			t.Fatalf("ClearPreviousIfMatches() error = %v", err)
		}
		got, _ := s.Previous()
		if got != "" {
			t.Errorf("Previous() = %q, want empty after matching clear", got)
		}
	})

	t.Run("no-op on mismatch", func(t *testing.T) {
		s := New(t.TempDir())
		if err := s.SetPrevious("staging"); err != nil {
			t.Fatal(err)
		}
		if err := s.ClearPreviousIfMatches("prod"); err != nil {
			t.Fatalf("ClearPreviousIfMatches() error = %v", err)
		}
		got, _ := s.Previous()
		if got != "staging" {
			t.Errorf("Previous() = %q, want staging unchanged", got)
		}
	})
}

func TestRenamePreviousIfMatches(t *testing.T) {
	t.Run("rewrites on match", func(t *testing.T) {
		s := New(t.TempDir())
		if err := s.SetPrevious("old"); err != nil {
			t.Fatal(err)
		}
		if err := s.RenamePreviousIfMatches("old", "new"); err != nil {
			t.Fatalf("RenamePreviousIfMatches() error = %v", err)
		}
		got, _ := s.Previous()
		if got != "new" {
			t.Errorf("Previous() = %q, want new", got)
		}
	})

	t.Run("no-op on mismatch", func(t *testing.T) {
		s := New(t.TempDir())
		if err := s.SetPrevious("other"); err != nil {
			t.Fatal(err)
		}
		if err := s.RenamePreviousIfMatches("old", "new"); err != nil {
			t.Fatalf("RenamePreviousIfMatches() error = %v", err)
		}
		got, _ := s.Previous()
		if got != "other" {
			t.Errorf("Previous() = %q, want other unchanged", got)
		}
	})

	t.Run("no-op when nothing recorded", func(t *testing.T) {
		s := New(t.TempDir())
		if err := s.RenamePreviousIfMatches("old", "new"); err != nil {
			t.Fatalf("RenamePreviousIfMatches() error = %v", err)
		}
		got, _ := s.Previous()
		if got != "" {
			t.Errorf("Previous() = %q, want empty", got)
		}
	})
}

func TestADC_saveLoadHasDelete(t *testing.T) {
	s := New(t.TempDir())

	has, err := s.HasADC("ctx")
	if err != nil {
		t.Fatalf("HasADC() error = %v", err)
	}
	if has {
		t.Error("HasADC() = true before any save, want false")
	}

	data := []byte(`{"type":"authorized_user"}`)
	if err := s.SaveADC("ctx", data); err != nil {
		t.Fatalf("SaveADC() error = %v", err)
	}

	has, err = s.HasADC("ctx")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("HasADC() = false after save, want true")
	}

	got, err := s.LoadADC("ctx")
	if err != nil {
		t.Fatalf("LoadADC() error = %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("LoadADC() = %q, want %q", got, data)
	}

	if runtime.GOOS != "windows" {
		path, err := s.ADCPath("ctx")
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("stored ADC file mode = %v, want 0600", perm)
		}
	}

	if err := s.DeleteADC("ctx"); err != nil {
		t.Fatalf("DeleteADC() error = %v", err)
	}
	has, err = s.HasADC("ctx")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("HasADC() = true after delete, want false")
	}
	// Deleting again must not error.
	if err := s.DeleteADC("ctx"); err != nil {
		t.Errorf("DeleteADC() (twice) error = %v", err)
	}
}

func TestADC_rename(t *testing.T) {
	s := New(t.TempDir())
	data := []byte(`{"type":"service_account"}`)
	if err := s.SaveADC("old", data); err != nil {
		t.Fatal(err)
	}

	if err := s.RenameADC("old", "new"); err != nil {
		t.Fatalf("RenameADC() error = %v", err)
	}
	if has, _ := s.HasADC("old"); has {
		t.Error("HasADC(old) = true after rename, want false")
	}
	got, err := s.LoadADC("new")
	if err != nil {
		t.Fatalf("LoadADC(new) error = %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("LoadADC(new) = %q, want %q", got, data)
	}
}

func TestADC_rename_noSourceIsNoop(t *testing.T) {
	s := New(t.TempDir())
	if err := s.RenameADC("ghost", "new"); err != nil {
		t.Fatalf("RenameADC() with no stored ADC error = %v, want nil (no-op)", err)
	}
	if has, _ := s.HasADC("new"); has {
		t.Error("HasADC(new) = true, want false (nothing to rename)")
	}
}

func TestADCPath(t *testing.T) {
	s := New("/config-dir")
	want := filepath.Join("/config-dir", "gcloud-ctx", "adc", "myctx.json")
	got, err := s.ADCPath("myctx")
	if err != nil {
		t.Fatalf("ADCPath(myctx) error = %v", err)
	}
	if got != want {
		t.Errorf("ADCPath(myctx) = %q, want %q", got, want)
	}
}

// badNames are the invalid-name attack surface every path-building method
// must reject: path traversal, an embedded traversal segment, empty, ".",
// and the reserved NONE pseudo-configuration.
var badNames = []string{"../../../x", "a/../b", "", ".", "NONE"}

func TestADCPath_rejectsInvalidNames(t *testing.T) {
	s := New("/config-dir")
	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			if _, err := s.ADCPath(name); err == nil {
				t.Errorf("ADCPath(%q) error = nil, want error", name)
			}
		})
	}
}

// TestStore_HasADC_none covers the one deliberate exception to "NONE is
// rejected everywhere": HasADC(NONE) must report false without erroring,
// since switching to NONE ("gcloud-ctx --unset") unconditionally asks
// whether there's a stored ADC to install, and NONE structurally never has
// one.
func TestStore_HasADC_none(t *testing.T) {
	s := New(t.TempDir())
	has, err := s.HasADC("NONE")
	if err != nil {
		t.Fatalf("HasADC(NONE) error = %v, want nil", err)
	}
	if has {
		t.Error("HasADC(NONE) = true, want false")
	}
}

func TestStore_ADCMethods_rejectInvalidNames(t *testing.T) {
	s := New(t.TempDir())
	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			if name != "NONE" { // HasADC(NONE) is the one deliberate exception; see TestStore_HasADC_none.
				if _, err := s.HasADC(name); err == nil {
					t.Errorf("HasADC(%q) error = nil, want error", name)
				}
			}
			if _, err := s.LoadADC(name); err == nil {
				t.Errorf("LoadADC(%q) error = nil, want error", name)
			}
			if err := s.SaveADC(name, []byte(`{}`)); err == nil {
				t.Errorf("SaveADC(%q) error = nil, want error", name)
			}
			if err := s.DeleteADC(name); err == nil {
				t.Errorf("DeleteADC(%q) error = nil, want error", name)
			}
			if err := s.RenameADC(name, "valid-name"); err == nil {
				t.Errorf("RenameADC(%q, valid-name) error = nil, want error", name)
			}
			if err := s.RenameADC("valid-name", name); err == nil {
				t.Errorf("RenameADC(valid-name, %q) error = nil, want error", name)
			}
		})
	}
	// None of those attempts may have created anything outside adcDir().
	entries, err := os.ReadDir(s.baseDir())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != adcSubdir {
			t.Errorf("unexpected entry %q created under gcloud-ctx state dir", e.Name())
		}
	}
}
