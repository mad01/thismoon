package history

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CleanResult reports what a Clean run did.
type CleanResult struct {
	Stats      Stats
	BackupPath string // empty on dry runs
	Refs       []string
}

// CheckPreconditions verifies the repo is safe to rewrite: not bare, on a
// branch, with a clean working tree.
func CheckPreconditions(repoPath string) error {
	out, err := gitOutput(repoPath, "rev-parse", "--is-bare-repository")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "true" {
		return fmt.Errorf("refusing to rewrite a bare repository")
	}
	if err := exec.Command("git", "-C", repoPath, "symbolic-ref", "-q", "HEAD").Run(); err != nil {
		return fmt.Errorf("refusing to rewrite with a detached HEAD; check out a branch first")
	}
	out, err = gitOutput(repoPath, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("working tree is dirty; commit or discard changes before rewriting history (stashes are refused too)")
	}
	// refs/stash is not among the rewritten refs, so a stashed secret would
	// survive a rewrite that reports success. Refuse rather than leak.
	out, err = gitOutput(repoPath, "stash", "list")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf(
			"stash is not empty; a stashed secret would survive the rewrite — " +
				"drop or apply your stashes first (git stash list)",
		)
	}
	// A branch checked out in another linked worktree would be left pointing
	// at rewritten history with a stale working tree; refuse rather than
	// silently desync it.
	out, err = gitOutput(repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return err
	}
	worktrees := 0
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			worktrees++
		}
	}
	if worktrees > 1 {
		return fmt.Errorf(
			"repository has other linked worktrees; their checkouts would be left on " +
				"stale history — remove them first (git worktree list)",
		)
	}
	return nil
}

// LocalRefs lists the branch and tag refs a rewrite will touch.
// Remote-tracking refs are deliberately excluded: rewriting them would
// desynchronize them from the actual remote.
func LocalRefs(repoPath string) ([]string, error) {
	out, err := gitOutput(
		repoPath, "for-each-ref", "refs/heads", "refs/tags", "--format=%(refname)",
	)
	if err != nil {
		return nil, err
	}
	var refs []string
	for ref := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("no local branches or tags to rewrite in %s", repoPath)
	}
	return refs, nil
}

// backup writes a bundle of all refs into .git so the rewrite is
// recoverable with `git clone <bundle>` or `git fetch <bundle>`.
func backup(repoPath string) (string, error) {
	gitDir, err := gitOutput(repoPath, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", err
	}
	path := filepath.Join(
		strings.TrimSpace(gitDir),
		fmt.Sprintf("suspenders-backup-%d.bundle", time.Now().Unix()),
	)
	cmd := exec.Command("git", "-C", repoPath, "bundle", "create", path, "--all")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git bundle create: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return path, nil
}

// DryRun exports history and runs the transform without importing anything,
// returning the stats a real Clean would produce.
func DryRun(repoPath string, rw *Rewriter) (CleanResult, error) {
	refs, err := LocalRefs(repoPath)
	if err != nil {
		return CleanResult{}, err
	}
	stats, err := runExportPipeline(repoPath, rw, refs, io.Discard)
	if err != nil {
		return CleanResult{}, err
	}
	return CleanResult{Stats: stats, Refs: refs}, nil
}

// Clean rewrites the repository's local branches and tags in place:
// backup bundle, then fast-export | Transform | fast-import, then a hard
// reset to sync the working tree. Callers own the confirmation gate.
func Clean(repoPath string, rw *Rewriter) (CleanResult, error) {
	if err := CheckPreconditions(repoPath); err != nil {
		return CleanResult{}, err
	}
	refs, err := LocalRefs(repoPath)
	if err != nil {
		return CleanResult{}, err
	}
	backupPath, err := backup(repoPath)
	if err != nil {
		return CleanResult{}, err
	}
	result := CleanResult{BackupPath: backupPath, Refs: refs}

	importCmd := exec.Command("git", "-C", repoPath, "fast-import", "--force", "--quiet")
	importIn, err := importCmd.StdinPipe()
	if err != nil {
		return result, fmt.Errorf("git fast-import pipe: %w", err)
	}
	var importErr bytes.Buffer
	importCmd.Stderr = &importErr
	if err := importCmd.Start(); err != nil {
		return result, fmt.Errorf("git fast-import: %w", err)
	}

	stats, exportErr := runExportPipeline(repoPath, rw, refs, importIn)
	result.Stats = stats
	closeErr := importIn.Close()
	waitErr := importCmd.Wait()
	if exportErr != nil {
		return result, exportErr
	}
	if closeErr != nil {
		return result, fmt.Errorf("close fast-import stream: %w", closeErr)
	}
	if waitErr != nil {
		removeFastImportCrashFiles(repoPath)
		return result, fmt.Errorf(
			"git fast-import: %w: %s", waitErr, strings.TrimSpace(importErr.String()),
		)
	}

	// fast-import has already moved the refs onto rewritten history. Sync the
	// working tree immediately; if that fails the refs are rewritten but the
	// tree still holds pre-rewrite content, so spell out the recovery.
	cmd := exec.Command("git", "-C", repoPath, "reset", "--hard")
	if out, err := cmd.CombinedOutput(); err != nil {
		return result, fmt.Errorf(
			"history was rewritten but the working tree could not be synced: "+
				"git reset --hard: %w: %s\n"+
				"  recover with: git -C %s reset --hard\n"+
				"  until then the working tree still holds pre-rewrite content — do not commit it",
			err, strings.TrimSpace(string(out)), repoPath,
		)
	}
	return result, nil
}

// removeFastImportCrashFiles deletes the crash dumps git fast-import leaves in
// the git dir on failure; each is an unredacted copy of the input stream.
func removeFastImportCrashFiles(repoPath string) {
	gitDir, err := gitOutput(repoPath, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return
	}
	matches, err := filepath.Glob(filepath.Join(strings.TrimSpace(gitDir), "fast_import_crash_*"))
	if err != nil {
		return
	}
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// runExportPipeline runs git fast-export over refs and streams it through
// the rewriter into w.
func runExportPipeline(
	repoPath string,
	rw *Rewriter,
	refs []string,
	w io.Writer,
) (Stats, error) {
	args := []string{
		"-C", repoPath, "fast-export",
		"--reencode=no", "--signed-tags=strip", "--tag-of-filtered-object=rewrite",
	}
	args = append(args, refs...)

	cmd := exec.Command("git", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Stats{}, fmt.Errorf("git fast-export pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Stats{}, fmt.Errorf("git fast-export: %w", err)
	}

	stats, transformErr := rw.Transform(stdout, w)
	waitErr := cmd.Wait()
	if transformErr != nil {
		return stats, transformErr
	}
	if waitErr != nil {
		return stats, fmt.Errorf(
			"git fast-export: %w: %s", waitErr, strings.TrimSpace(stderr.String()),
		)
	}
	return stats, nil
}
