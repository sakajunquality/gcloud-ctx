package ui

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ktr0731/go-fuzzyfinder"
)

// ErrCancelled indicates the user cancelled the interactive picker (Esc or
// Ctrl-C) without selecting anything.
var ErrCancelled = errors.New("picker cancelled")

// PickerItem is one entry offered by Pick: Name is the selectable/filterable
// text, Preview is shown in the preview pane when the item is highlighted.
type PickerItem struct {
	Name    string
	Preview string
}

// PickerEnabled reports whether the interactive picker should be used
// instead of a plain list: both in and out must be terminals, and
// GCLOUD_CTX_IGNORE_FZF must be unset.
func PickerEnabled(in io.Reader, out io.Writer) bool {
	if os.Getenv("GCLOUD_CTX_IGNORE_FZF") != "" {
		return false
	}
	return IsTerminalReader(in) && IsTerminal(out)
}

// Pick runs the interactive fuzzy picker over items and returns the Name of
// the selected one. It returns ErrCancelled if the user cancels without
// selecting anything.
func Pick(items []PickerItem) (string, error) {
	idx, err := fuzzyfinder.Find(
		items,
		func(i int) string { return items[i].Name },
		fuzzyfinder.WithPreviewWindow(func(i, _, _ int) string {
			if i < 0 || i >= len(items) {
				return ""
			}
			return items[i].Preview
		}),
	)
	if err != nil {
		if errors.Is(err, fuzzyfinder.ErrAbort) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("run picker: %w", err)
	}
	return items[idx].Name, nil
}
