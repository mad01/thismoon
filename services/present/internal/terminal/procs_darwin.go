//go:build darwin

package terminal

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// sZomb is SZOMB from <sys/proc.h>: a process that has exited and waits to
// be reaped. It can take no signal, so it counts as gone.
const sZomb = 5

// listAttempts bounds the reads of the process list: the sysctl measures
// the list and then copies it, and fails with ENOMEM when processes
// started in between outgrow the room it measured.
const listAttempts = 3

// sessionGroups lists the process groups of every live process this user
// runs in session sid. macOS keeps no sysctl by session, so it lists the
// user's processes and asks each for its session. ok is false when the
// process list cannot be read; a kill then falls back to the groups the
// terminal names.
func sessionGroups(sid int) (groups []int, ok bool) {
	var procs []unix.KinfoProc
	var err error
	for range listAttempts {
		procs, err = unix.SysctlKinfoProcSlice("kern.proc.uid", os.Geteuid())
		if !errors.Is(err, unix.ENOMEM) {
			break
		}
	}
	if err != nil {
		return nil, false
	}
	seen := map[int]bool{}
	for i := range procs {
		p := &procs[i]
		if p.Proc.P_stat == sZomb {
			continue
		}
		// An error means the process is already gone.
		if got, err := unix.Getsid(int(p.Proc.P_pid)); err != nil || got != sid {
			continue
		}
		if g := int(p.Eproc.Pgid); !seen[g] {
			seen[g] = true
			groups = append(groups, g)
		}
	}
	return groups, true
}
