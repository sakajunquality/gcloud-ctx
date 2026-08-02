package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestList_plain(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "")
	writeFile(t, filepath.Join(dir, "configurations", "config_staging"), "")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	// GCLOUD_CTX_IGNORE_FZF is set by testEnv, and the buffers used by run()
	// are never terminals either way, so this always exercises the plain
	// (non-picker) path — no ANSI color codes, since the output isn't a TTY.
	stdout, _, err := run(t)
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "default\nstaging\n" {
		t.Errorf("stdout = %q, want plain sorted names, one per line, no color codes", stdout)
	}
}

func TestList_empty(t *testing.T) {
	testEnv(t)
	stdout, stderr, err := run(t)
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "No contexts found") {
		t.Errorf("stderr = %q, want a no-contexts hint", stderr)
	}
}

func TestList_long(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "[core]\naccount = a@example.com\nproject = proj-a\n")
	writeFile(t, filepath.Join(dir, "active_config"), "default")

	stdout, _, err := run(t, "-l")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "NAME") || !strings.Contains(stdout, "ACTIVE") || !strings.Contains(stdout, "ADC") {
		t.Errorf("stdout = %q, want a header row with NAME/ACTIVE/ADC columns", stdout)
	}
	if !strings.Contains(stdout, "default") || !strings.Contains(stdout, "a@example.com") || !strings.Contains(stdout, "proj-a") {
		t.Errorf("stdout = %q, want the default row with its properties", stdout)
	}
	if !strings.Contains(stdout, "*") {
		t.Errorf("stdout = %q, want an active marker", stdout)
	}
}

// TestList_long_colorForced_staysAligned covers item 9: column widths must
// be computed from visible text, with color spliced in afterward — with
// FORCE_COLOR on, the raw ANSI bytes must never appear inside the padding,
// and every data row's columns must still line up with the header.
func TestList_long_colorForced_staysAligned(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_alpha"), "[core]\naccount = a@example.com\nproject = proj-a\n")
	writeFile(t, filepath.Join(dir, "configurations", "config_zzz-long-name"), "[core]\naccount = b@example.com\n")
	writeFile(t, filepath.Join(dir, "active_config"), "alpha")
	t.Setenv("FORCE_COLOR", "1")

	stdout, _, err := run(t, "-l")
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 3 { // header + alpha + zzz-long-name
		t.Fatalf("stdout = %q, want 3 lines (header + 2 rows)", stdout)
	}
	header, alphaLine, zzzLine := lines[0], lines[1], lines[2]

	if !strings.Contains(alphaLine, "\x1b[") {
		t.Errorf("active row = %q, want ANSI color codes present", alphaLine)
	}

	// The ACTIVE/ACCOUNT/PROJECT/... columns must start at the same visible
	// column on every row: strip ANSI codes and compare the position of the
	// "*" active marker (the second column) against the header's ACTIVE
	// column start. If color codes had been fed into tabwriter, the longer
	// "zzz-long-name" row would have pushed the padding for its own row only,
	// but the *colored* row's padding is what's under test: color must not
	// have shortened its column below the others.
	stripANSI := func(s string) string {
		var b strings.Builder
		inEsc := false
		for _, r := range s {
			switch {
			case r == '\x1b':
				inEsc = true
			case inEsc && r == 'm':
				inEsc = false
			case !inEsc:
				b.WriteRune(r)
			}
		}
		return b.String()
	}

	plainAlpha := stripANSI(alphaLine)
	plainZzz := stripANSI(zzzLine)

	activeCol := strings.Index(header, "ACTIVE")
	if strings.Index(plainAlpha, "*") != activeCol {
		t.Errorf("plain (color-stripped) active row = %q, want '*' aligned with header ACTIVE column (%d): got %d", plainAlpha, activeCol, strings.Index(plainAlpha, "*"))
	}
	// Both rows' ACCOUNT column (right after ACTIVE) must start at the same
	// visible offset once color is stripped, proving alignment didn't depend
	// on whether the row was colorized.
	accountCol := strings.Index(header, "ACCOUNT")
	nonSpace := func(s string, from int) int {
		for i := from; i < len(s); i++ {
			if s[i] != ' ' {
				return i
			}
		}
		return -1
	}
	if got := nonSpace(plainAlpha, activeCol+1); got != accountCol {
		t.Errorf("alpha row ACCOUNT starts at %d, want %d (header-aligned)", got, accountCol)
	}
	if got := nonSpace(plainZzz, activeCol+1); got != accountCol {
		t.Errorf("zzz row ACCOUNT starts at %d, want %d (header-aligned)", got, accountCol)
	}
}

// TestList_long_sanitizesControlChars covers item 9's list side: a property
// value containing a C0 control character must not be echoed verbatim into
// the table.
func TestList_long_sanitizesControlChars(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_default"), "[core]\nproject = proj\x07ect\n")

	stdout, _, err := run(t, "-l")
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

// TestList_long_corruptConfig_rendersParseErrorMarker covers item 10: a
// config file that fails to parse must not abort "-l" — it renders with a
// parse-error marker in its property columns, ADC status still computed.
func TestList_long_corruptConfig_rendersParseErrorMarker(t *testing.T) {
	dir := testEnv(t)
	// AllowPythonMultilineValues / IgnoreInlineComment tolerate a lot, but a
	// value line with no preceding key is a genuine INI parse error.
	writeFile(t, filepath.Join(dir, "configurations", "config_broken"), "not an ini file\n=== garbage ===\n[[[")
	writeFile(t, filepath.Join(dir, "configurations", "config_fine"), "[core]\naccount = a@example.com\n")

	stdout, _, err := run(t, "-l")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "broken") {
		t.Errorf("stdout = %q, want the corrupt config's name still listed", stdout)
	}
	if !strings.Contains(stdout, parseErrorMarker) {
		t.Errorf("stdout = %q, want a parse-error marker for the corrupt config", stdout)
	}
	if !strings.Contains(stdout, "fine") || !strings.Contains(stdout, "a@example.com") {
		t.Errorf("stdout = %q, want the other (valid) config to render normally", stdout)
	}
}

// TestList_plain_corruptConfig_stillPrintsName covers item 10's plain-list
// side: the bare (non -l) listing must keep working too, printing just the
// name for a config whose file fails to parse.
func TestList_plain_corruptConfig_stillPrintsName(t *testing.T) {
	dir := testEnv(t)
	writeFile(t, filepath.Join(dir, "configurations", "config_broken"), "not an ini file\n=== garbage ===\n[[[")

	stdout, _, err := run(t)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout) != "broken" {
		t.Errorf("stdout = %q, want just the name %q", stdout, "broken")
	}
}
