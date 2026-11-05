package lockfile

import (
	"path/filepath"
	"testing"
	"time"
)

func TestHoldExcludesASecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locks", "v.index.lock")
	release, err := Hold(path)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan func() error)
	go func() {
		second, err := Hold(path)
		if err != nil {
			t.Error(err)
			close(acquired)
			return
		}
		acquired <- second
	}()
	select {
	case <-acquired:
		t.Fatal("second Hold acquired the lock while the first held it")
	case <-time.After(100 * time.Millisecond):
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	select {
	case second := <-acquired:
		if second == nil {
			t.Fatal("second Hold failed")
		}
		if err := second(); err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second Hold never acquired the released lock")
	}
}
