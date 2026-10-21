package settings

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mad01/thismoon/tools/agent-sync/internal/apply"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

// hardlinkTarget stands in for a link target when the live file is a hard
// link, which has no target to print.
const hardlinkTarget = "(hardlink)"

// freshMode is the mode of a live file sync creates. Settings can carry env
// secrets, so only the owner reads it; Claude Code creates it 0600 too.
const freshMode fs.FileMode = 0o600

// Run merges the template (and overlay) into the live file as mode says,
// printing the report to out.Out and warnings to out.Err. Check prints a
// one-line summary first and returns apply.Exit(apply.CodeDrift) on drift.
func Run(cfg Config, mode apply.Mode, out apply.Streams) error {
	return runner{cfg: cfg, mode: mode, out: out.Out, warn: out.Err}.run()
}

// runner is one Run: the config, the mode, and where the report goes.
type runner struct {
	cfg  Config
	mode apply.Mode
	out  io.Writer
	warn io.Writer
}

func (r runner) run() error {
	template, err := readTemplate(r.cfg.Template)
	if err != nil {
		return err
	}
	if conflicts := pruneConflicts(template, r.cfg.Prune); len(conflicts) > 0 {
		names := make([]string, len(conflicts))
		for i, p := range conflicts {
			names[i] = p.String()
		}
		return fmt.Errorf("settings.prune retires keys the template still sets: %s "+
			"(drop them from the template or from settings.prune)", strings.Join(names, ", "))
	}
	found, err := lexists(r.cfg.Live)
	if err != nil {
		return fmt.Errorf("live settings %s: %w", r.cfg.Live, err)
	}
	if !found {
		return r.writeFresh(template)
	}
	link, err := linkTarget(r.cfg.Live)
	if err != nil {
		return err
	}
	if link != "" {
		if r.mode == apply.Check {
			fmt.Fprintf(r.out, "drift: %s is a symlink %s %s (needs migration to a merged file)\n",
				r.cfg.Live, arrow, link)
			return apply.Exit(apply.CodeDrift)
		}
		fmt.Fprintf(r.out, "note: %s is a symlink %s %s\n", r.cfg.Live, arrow, link)
		fmt.Fprintln(r.out, breakLinkLine)
	}
	// A dangling symlink has nothing to merge from, so it is replaced the
	// way a missing file is created. Its target is never created.
	resolves, err := exists(r.cfg.Live)
	if err != nil {
		return fmt.Errorf("live settings %s: %w", r.cfg.Live, err)
	}
	if !resolves {
		return r.writeFresh(template)
	}
	live, err := readObject(r.cfg.Live, "live settings")
	if err != nil {
		return err
	}
	return r.merge(template, live, link != "")
}

// merge prunes, merges and reports, then writes the live file in Sync.
// linked says the live file is a link to break. The diff compares against
// the unpruned live file, so a pruned key counts as drift.
func (r runner) merge(template, live *pyjson.Object, linked bool) error {
	prunedLive := prune(live, r.cfg.Prune)
	merged := deepMerge(prunedLive, template, nil, nil)
	// expected is template plus overlay, so keys the overlay sets are not
	// reported as local-only.
	expected := template
	overlay, err := r.overlay()
	if err != nil {
		return err
	}
	if overlay != nil {
		merged = mergeOverlay(merged, overlay)
		expected = mergeOverlay(template, overlay)
	}

	diffs := collectDiff(live, merged, nil)
	if len(diffs) == 0 {
		fmt.Fprintln(r.out, inSyncLine)
		// sync_settings.py unlinks a linked live file before it merges, so
		// a link already in sync was deleted and never rewritten. Write the
		// regular file it promised instead.
		if linked && r.mode.Writes() {
			return writeLive(r.cfg.Live, merged)
		}
		return nil
	}
	if r.mode == apply.Check {
		fmt.Fprintln(r.out, driftSummary(len(diffs)))
		return apply.Exit(apply.CodeDrift)
	}
	writeSections(r.out, diffs, r.cfg.Prune, findLocalOnly(prunedLive, expected, ""))
	if !r.mode.Writes() {
		fmt.Fprintln(r.out, planFooter)
		return nil
	}
	if err := writeLive(r.cfg.Live, merged); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "\nwrote %s\n", r.cfg.Live)
	return nil
}

// writeFresh handles a live path with nothing to merge from: no file yet,
// or a dangling symlink. Check reports drift, Plan says what it would
// write, Sync writes the template with the overlay merged on top, so the
// first sync on a new machine leaves nothing for the next one to do.
func (r runner) writeFresh(template *pyjson.Object) error {
	live := r.cfg.Live
	if r.mode == apply.Check {
		fmt.Fprintf(r.out, "drift: %s does not exist yet\n", live)
		return apply.Exit(apply.CodeDrift)
	}
	doc := template
	overlay, err := r.overlay()
	if err != nil {
		return err
	}
	if overlay != nil {
		doc = mergeOverlay(template, overlay)
	}
	if !r.mode.Writes() {
		fmt.Fprintf(r.out, "(dry-run) would write new %s from template\n", live)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(live), 0o777); err != nil {
		return fmt.Errorf("live settings: %w", err)
	}
	if err := writeLive(live, doc); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "wrote new %s from template (no prior file)\n", live)
	return nil
}

// overlay reads the overlay and, outside Check, prints the line saying
// it is merged. Nil means there is no overlay to merge.
func (r runner) overlay() (*pyjson.Object, error) {
	overlay, err := readOverlay(r.cfg.Overlay, r.warn)
	if err != nil || overlay == nil {
		return nil, err
	}
	if r.mode != apply.Check {
		fmt.Fprintf(r.out, "merged local overlay: %s\n", r.cfg.Overlay)
	}
	return overlay, nil
}

// writeLive writes doc to path as json.dumps(doc, indent=2) plus a newline.
// The bytes go to a temp file beside path that is then renamed over it, so
// a reader never sees a partial file, a failed write leaves path as it was,
// and a link at path is replaced by a regular file instead of written
// through. The file keeps the mode of what path resolves to (a link's
// target), or gets freshMode when path resolves to nothing.
func writeLive(path string, doc *pyjson.Object) error {
	mode, err := liveMode(path)
	if err != nil {
		return fmt.Errorf("live settings: %w", err)
	}
	data := pyjson.Indent(doc, 2) + "\n"
	if err := writeAtomic(path, []byte(data), mode); err != nil {
		return fmt.Errorf("live settings: %w", err)
	}
	return nil
}

// liveMode returns the permission bits of what path resolves to, or
// freshMode when it resolves to nothing.
func liveMode(path string) (fs.FileMode, error) {
	info, err := os.Stat(path)
	ok, err := present(info, err)
	if err != nil {
		return 0, err
	}
	if !ok {
		return freshMode, nil
	}
	return info.Mode().Perm(), nil
}

// writeAtomic writes data to a temp file in path's directory with mode,
// then renames it to path. On any failure the temp file is removed and
// path is untouched.
func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(mode)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		return errors.Join(err, os.Remove(tmp.Name()))
	}
	return nil
}

// readTemplate reads the template, which must exist and hold an object.
func readTemplate(path string) (*pyjson.Object, error) {
	present, err := exists(path)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", path, err)
	}
	if !present {
		return nil, fmt.Errorf("template not found: %s", path)
	}
	return readObject(path, "template")
}

// readOverlay reads the overlay. A missing overlay is nil with no error. One
// that cannot be read or parsed is skipped with a warning, as the Python
// script skipped it; one that is not UTF-8 or not an object is an error.
func readOverlay(path string, stderr io.Writer) (*pyjson.Object, error) {
	present, err := exists(path)
	if err != nil {
		return nil, fmt.Errorf("overlay %s: %w", path, err)
	}
	if !present {
		return nil, nil
	}
	v, err := pyjson.ReadFile(path)
	var syntax *pyjson.SyntaxError
	var pathErr *fs.PathError
	switch {
	case errors.As(err, &syntax) || errors.As(err, &pathErr):
		apply.Warn(stderr, "skipping %s: %v", path, err)
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("overlay %s: %w", path, err)
	}
	return asObject(v, "overlay", path)
}

// readObject reads a JSON file that must hold an object at the top level.
func readObject(path, what string) (*pyjson.Object, error) {
	v, err := pyjson.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", what, path, err)
	}
	return asObject(v, what, path)
}

func asObject(v any, what, path string) (*pyjson.Object, error) {
	obj, ok := v.(*pyjson.Object)
	if !ok {
		return nil, fmt.Errorf("%s %s: want a JSON object at the top level, got %s",
			what, path, pyjson.Describe(v))
	}
	return obj, nil
}

// exists reports whether path names something, following symlinks the way
// Python's Path.exists does: a dangling link counts as absent.
func exists(path string) (bool, error) { return present(os.Stat(path)) }

// lexists reports whether path names something without following a
// symlink at path: a dangling link counts as present.
func lexists(path string) (bool, error) { return present(os.Lstat(path)) }

// present reads a stat result as presence: a missing file, a link loop or a
// non-directory parent count as absent rather than as errors.
func present(_ fs.FileInfo, err error) (bool, error) {
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR),
		errors.Is(err, syscall.ELOOP):
		return false, nil
	default:
		return false, err
	}
}

// linkTarget returns what path links to: the symlink's target text,
// hardlinkTarget for a file with more than one hard link, or "" for a plain
// regular file.
func linkTarget(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("live settings: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return "", fmt.Errorf("live settings: %w", err)
		}
		return target, nil
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Nlink > 1 {
		return hardlinkTarget, nil
	}
	return "", nil
}
