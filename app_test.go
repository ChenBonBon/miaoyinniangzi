package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPickerPrefersDraftAndFallsBack(t *testing.T) {
	saved := t.TempDir()
	draft := t.TempDir()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ draft, want string }{{draft, draft}, {"relative", saved}, {filepath.Join(draft, "missing"), saved}, {file, saved}, {"", saved}} {
		if got := pickerDirectory(tc.draft, saved); got != tc.want {
			t.Fatalf("draft %q: got %q, want %q", tc.draft, got, tc.want)
		}
	}
	if got := pickerDirectory("relative"); got != "" {
		t.Fatalf("invalid path accepted: %q", got)
	}
}
