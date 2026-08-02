package ui

import (
	"bytes"
	"testing"
)

func TestDetectStyle(t *testing.T) {
	tests := []struct {
		name           string
		forceColor     string
		noColor        string
		wantActiveText string
	}{
		{name: "non-terminal, no env", wantActiveText: "x"},
		{name: "non-terminal, NO_COLOR set", noColor: "1", wantActiveText: "x"},
		{name: "non-terminal, FORCE_COLOR set", forceColor: "1", wantActiveText: ansiBoldYellow + "x" + ansiReset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FORCE_COLOR", tt.forceColor)
			t.Setenv("NO_COLOR", tt.noColor)

			var buf bytes.Buffer // never a terminal
			s := DetectStyle(&buf)
			if got := s.Active("x"); got != tt.wantActiveText {
				t.Errorf("Active(%q) = %q, want %q", "x", got, tt.wantActiveText)
			}
		})
	}
}

func TestIsTerminal_nonFile(t *testing.T) {
	var buf bytes.Buffer
	if IsTerminal(&buf) {
		t.Error("IsTerminal(bytes.Buffer) = true, want false")
	}
	if IsTerminalReader(&buf) {
		t.Error("IsTerminalReader(bytes.Buffer) = true, want false")
	}
}
