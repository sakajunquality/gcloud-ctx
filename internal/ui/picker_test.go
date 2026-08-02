package ui

import (
	"bytes"
	"testing"
)

func TestPickerEnabled(t *testing.T) {
	var in, out bytes.Buffer // never terminals

	t.Setenv("GCLOUD_CTX_IGNORE_FZF", "")
	if PickerEnabled(&in, &out) {
		t.Error("PickerEnabled with non-terminal streams = true, want false")
	}

	t.Setenv("GCLOUD_CTX_IGNORE_FZF", "1")
	if PickerEnabled(&in, &out) {
		t.Error("PickerEnabled with GCLOUD_CTX_IGNORE_FZF set = true, want false")
	}
}
