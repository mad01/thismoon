package mcpserver

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestDetectFileError(t *testing.T) {
	t.Run("permission denial gets sandbox guidance", func(t *testing.T) {
		stat := fmt.Errorf("stat /Users/x/secret.env: %w", fs.ErrPermission)
		got := detectFileError(stat, "/Users/x/secret.env")
		if !strings.Contains(got.Error(), "humanizer_detect instead") {
			t.Errorf("want guidance pointing at humanizer_detect, got: %v", got)
		}
		if !errors.Is(got, fs.ErrPermission) {
			t.Errorf("original error should stay wrapped, got: %v", got)
		}
	})

	t.Run("other errors pass through unchanged", func(t *testing.T) {
		notExist := fmt.Errorf("stat /tmp/gone.md: %w", fs.ErrNotExist)
		if got := detectFileError(notExist, "/tmp/gone.md"); got != notExist {
			t.Errorf("want error unchanged, got: %v", got)
		}
	})
}
