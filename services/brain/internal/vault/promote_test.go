package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPromoteRestoresEveryNoteWhenARewriteFails(t *testing.T) {
	files := map[string]string{
		"inbox/topic.md": "---\ntitle: Topic\nstatus: raw\n---\nCaptured.\n",
		"MOCs/maps.md":   "# Maps\n",
	}
	for i := 1; i <= 5; i++ {
		files[fmt.Sprintf("notes/link-%d.md", i)] = fmt.Sprintf(
			"---\ntitle: Link %d\n---\nSee [[inbox/topic]] and [[Topic]].\n", i)
	}
	v := newTestVault(t, files)
	injected := errors.New("disk full")
	writes := 0
	failThird := func(path string, data []byte) error {
		if strings.Contains(path, "link-") {
			writes++
			if writes == 3 {
				return injected
			}
		}
		return writeNoteFile(path, data)
	}
	_, err := v.promote(filepath.Join(v.Root, "inbox", "topic.md"),
		PromoteOptions{MOC: "MOCs/maps.md", Now: time.Now()}, failThird)
	if !errors.Is(err, injected) || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("err = %v, want the injected failure and a full rollback", err)
	}
	if writes != 3 {
		t.Errorf("wrote %d linking notes, want the failure on the third", writes)
	}
	for rel, want := range files {
		if got, err := os.ReadFile(filepath.Join(v.Root, rel)); err != nil || string(got) != want {
			t.Errorf("%s = %q (%v), want its old bytes %q", rel, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(v.Root, "notes", "topic.md")); !os.IsNotExist(err) {
		t.Errorf("the promoted note is still there: %v", err)
	}
}

func TestPromoteRollsBackAFailedMOC(t *testing.T) {
	files := map[string]string{
		"inbox/topic.md": "---\ntitle: Topic\nstatus: raw\n---\nCaptured.\n",
		"notes/link.md":  "---\ntitle: Link\n---\nSee [[inbox/topic]].\n",
	}
	v := newTestVault(t, files)
	failMOC := func(path string, data []byte) error {
		if strings.Contains(path, "MOCs") {
			_ = writeNoteFile(path, []byte("half"))
			return errors.New("injected")
		}
		return writeNoteFile(path, data)
	}
	_, err := v.promote(filepath.Join(v.Root, "inbox", "topic.md"),
		PromoteOptions{MOC: "MOCs/new.md", Now: time.Now()}, failMOC)
	if err == nil {
		t.Fatal("promote succeeded, want the injected MOC failure")
	}
	for rel, want := range files {
		if got, _ := os.ReadFile(filepath.Join(v.Root, rel)); string(got) != want {
			t.Errorf("%s = %q, want its old bytes", rel, got)
		}
	}
	if _, err := os.Stat(filepath.Join(v.Root, "MOCs", "new.md")); !os.IsNotExist(err) {
		t.Errorf("the MOC the promote created is still there: %v", err)
	}
}
