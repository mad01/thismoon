//go:build !darwin

package terminal

// sessionGroups is not implemented off macOS, the one platform present
// targets, so it never lists; a kill falls back to the shell's group and
// the foreground job.
func sessionGroups(int) (groups []int, ok bool) {
	return nil, false
}
