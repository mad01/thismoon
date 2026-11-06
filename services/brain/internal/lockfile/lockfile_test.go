package lockfile

import (
	"errors"
	"path/filepath"
	"strings"
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

func TestHoldTimeoutGivesUpNamingTheLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locks", "v.literal.lock")
	release, err := Hold(path)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	second, err := HoldTimeout(path, 150*time.Millisecond)
	if err == nil {
		_ = second()
		t.Fatal("HoldTimeout took a lock another holder has")
	}
	if !errors.Is(err, ErrTimeout) || !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want ErrTimeout naming %s", err, path)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond || elapsed > 3*time.Second {
		t.Errorf("HoldTimeout waited %s, want about 150ms", elapsed)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	third, err := HoldTimeout(path, time.Second)
	if err != nil {
		t.Fatalf("HoldTimeout of a free lock: %v", err)
	}
	if err := third(); err != nil {
		t.Fatal(err)
	}
}

func TestHoldTimeoutWaitsForARelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.write.lock")
	release, err := Hold(path)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(100*time.Millisecond, func() { _ = release() })
	second, err := HoldTimeout(path, 5*time.Second)
	if err != nil {
		t.Fatalf("HoldTimeout did not get the released lock: %v", err)
	}
	if err := second(); err != nil {
		t.Fatal(err)
	}
}
