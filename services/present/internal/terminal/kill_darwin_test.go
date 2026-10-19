//go:build darwin

package terminal

import (
	"regexp"
	"strconv"
	"testing"
	"time"
)

// Kill ends every process in the session, wherever its shell left it: a
// background job ignoring the hangup, which the kernel cuts off from the
// revoked terminal once the shell is gone, and a job that started only
// after the kill did.
func TestKillLeavesNoProcessInTheSession(t *testing.T) {
	cases := []struct {
		name, cmd, ready string
		job              bool // the output names the job's pid as bg=<pid>
	}{
		{
			"background job ignoring hangup",
			`sh -c 'trap "" HUP; echo "bg=$$ end"; exec sleep 65' & printf up; sleep 60`,
			" end", true,
		},
		{
			"job starting as the kill does",
			`printf up; sh -c 'sleep 0.3; trap "" HUP; exec sleep 64'; true`, "up", false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, _ := newManager(t, Config{KillGrace: 500 * time.Millisecond})
			sub := attach(t, m, "demo", Spec{Cmd: c.cmd})
			out := readUntil(t, sub, c.ready)
			sid := m.List()[0].PID

			if err := m.Kill("page1", "demo"); err != nil {
				t.Fatalf("Kill: %v", err)
			}
			waitFor(t, "the session to empty", func() bool {
				left, ok := sessionGroups(sid)
				if !ok {
					t.Fatal("sessionGroups could not list the processes")
				}
				return len(left) == 0
			})
			if !c.job {
				return
			}
			match := regexp.MustCompile(`bg=(\d+)`).FindStringSubmatch(out)
			if match == nil {
				t.Fatalf("no job pid in %q", out)
			}
			pid, _ := strconv.Atoi(match[1])
			// A job killed last can stay a zombie for a moment after the
			// session reads as empty, until launchd reaps it.
			waitFor(t, "the background job to be gone", func() bool { return groupGone(pid) })
		})
	}
}

// sessionGroups is what a kill trusts to find every job, so it must see
// the shell and a background job, and nothing once they are gone.
func TestSessionGroupsListsTheShellAndItsJobs(t *testing.T) {
	m, _ := newManager(t, Config{})
	// The shell prints a job notice of its own first, so the pid line ends
	// with a marker to wait for.
	sub := attach(t, m, "demo", Spec{Cmd: `sleep 61 & echo "bg=$! end"; sleep 60`})
	out := readUntil(t, sub, " end")
	match := regexp.MustCompile(`bg=(\d+)`).FindStringSubmatch(out)
	if match == nil {
		t.Fatalf("no background pid in %q", out)
	}
	bg, _ := strconv.Atoi(match[1])
	sid := m.List()[0].PID

	groups, ok := sessionGroups(sid)
	if !ok {
		t.Fatal("sessionGroups could not list the processes")
	}
	if !containsAll(groups, sid, bg) {
		t.Errorf("sessionGroups(%d) = %v, want the shell's group %d and the job's %d",
			sid, groups, sid, bg)
	}
	if err := m.Kill("page1", "demo"); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	waitFor(t, "the session to empty", func() bool {
		left, ok := sessionGroups(sid)
		return ok && len(left) == 0
	})
}

func containsAll(have []int, want ...int) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			found = found || h == w
		}
		if !found {
			return false
		}
	}
	return true
}
