package state

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// envLockHelper names the state dir TestLockHelperProcess locks a session
// in. Only the test binary that TestLockReleasesWhenTheHolderDies starts
// sets it.
const envLockHelper = "HARVEST_TEST_LOCK_HELPER_DIR"

func TestLock(t *testing.T) {
	d := newDir(t)
	release, err := d.Lock("s1")
	if err != nil {
		t.Fatalf("Lock() error = %v", err)
	}
	if _, err := d.Lock("s1"); !errors.Is(err, ErrRunning) {
		t.Errorf("a second Lock() of s1 error = %v, want ErrRunning", err)
	}
	other, err := d.Lock("s2")
	if err != nil {
		t.Errorf("Lock() of another session error = %v, want nil", err)
	} else {
		other()
	}
	release()
	again, err := d.Lock("s1")
	if err != nil {
		t.Fatalf("Lock() after release error = %v", err)
	}
	again()
}

func TestLockRejectsBadSession(t *testing.T) {
	d := newDir(t)
	for _, id := range []string{"", "..", "a/b", "a\nb"} {
		if _, err := d.Lock(id); !errors.Is(err, ErrBadSession) {
			t.Errorf("Lock(%q) error = %v, want ErrBadSession", id, err)
		}
	}
}

// TestLockReleasesWhenTheHolderDies starts a process that takes the lock and
// keeps it, checks that the lock holds across processes, then kills the
// holder: a run that was killed must not block the next one.
func TestLockReleasesWhenTheHolderDies(t *testing.T) {
	d := newDir(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	cmd.Env = append(os.Environ(), envLockHelper+"="+d.Path())
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("the holder said %q, %v; want locked", line, err)
	}

	if _, err := d.Lock("s1"); !errors.Is(err, ErrRunning) {
		t.Fatalf("Lock() while another process holds it: error = %v, want ErrRunning", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	release, err := d.Lock("s1")
	if err != nil {
		t.Fatalf("Lock() after the holder died: error = %v, want nil", err)
	}
	release()
}

// TestLockHelperProcess is the holder TestLockReleasesWhenTheHolderDies
// starts; on its own it does nothing.
func TestLockHelperProcess(t *testing.T) {
	dir := os.Getenv(envLockHelper)
	if dir == "" {
		t.Skip("the lock holder of TestLockReleasesWhenTheHolderDies")
	}
	if _, err := New(dir, time.Now).Lock("s1"); err != nil {
		t.Fatal(err)
	}
	fmt.Println("locked")
	time.Sleep(time.Minute) // until the test kills it
}

func TestSweepJobs(t *testing.T) {
	d := newDir(t)
	if err := d.SweepJobs(); err != nil {
		t.Fatalf("SweepJobs() with no jobs dir: error = %v", err)
	}
	for _, s := range []string{"killed", "busy"} {
		job, err := d.Job(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := job.Reset(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(job.User(), []byte("session text"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	release, err := d.Lock("busy")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	if err := d.SweepJobs(); err != nil {
		t.Fatalf("SweepJobs() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.Path(), "jobs", "killed")); !errors.Is(
		err,
		os.ErrNotExist,
	) {
		t.Errorf("the job dir no run holds is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.Path(), "jobs", "busy", "user.md")); err != nil {
		t.Errorf("the sweep removed a held job dir: %v", err)
	}
	// The sweep released the lock it took to remove the dir.
	again, err := d.Lock("killed")
	if err != nil {
		t.Fatalf("Lock() of a swept session error = %v", err)
	}
	again()
}
