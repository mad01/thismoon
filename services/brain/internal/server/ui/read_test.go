package ui_test

import (
	"io/fs"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/server/ui"
)

// read returns one embedded UI file as a string.
func read(t *testing.T, name string) string {
	t.Helper()
	b, err := fs.ReadFile(ui.Files(), name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}
