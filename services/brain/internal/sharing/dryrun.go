package sharing

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/check"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Verdicts of a dry run.
const (
	// VerdictProceed: the real promote would open its pull request.
	VerdictProceed = "proceed"
	// VerdictRefused: a gate refuses the note.
	VerdictRefused = "refused"
)

// PromotePlan is the JSON result of brain promote --to --dry-run: every
// gate the real promote runs, with nothing changed.
type PromotePlan struct {
	DryRun  bool   `json:"dry_run"`
	Verdict string `json:"verdict"`
	Vault   string `json:"vault"`
	Path    string `json:"path"`
	ID      string `json:"id,omitempty"`
	// SharedVault and Target are where the shared copy would go: the
	// vault, and the copy as a vault:path ref under the same path.
	SharedVault string `json:"shared_vault"`
	Target      string `json:"target"`
	// Branch is the pull request's branch name. The real promote adds a
	// -2, -3 suffix when the remote has the name already.
	Branch string `json:"branch,omitempty"`
	// Base is the remote branch the pull request would go to.
	Base     string   `json:"base,omitempty"`
	Gates    []Gate   `json:"gates"`
	MOCs     []string `json:"mocs,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Err returns the refusal the real promote would fail with, nil when it
// would proceed. Its sentinel gives the real promote's exit code.
func (p PromotePlan) Err() error {
	err := refusal(p.Gates)
	if err == nil {
		return nil
	}
	return fmt.Errorf("dry run: %s:%s would be refused: %w", p.Vault, p.Path, err)
}

// DryRun runs every gate Promote runs, against the shared clone as it is
// on disk, and changes nothing: no fetch, worktree, commit, push or gh,
// and no write to either vault or the state directory. It reads the
// clone's git config to find the pull request's remote. brain check reads
// notes from disk, so the copy is checked in a mirror of the clone's notes
// and policy in a temporary directory, removed before DryRun returns.
// Unlike Promote it carries on past a refused gate, so the plan shows
// every gate; only a refused target stops it.
func DryRun(
	lib *library.Library,
	ref vault.Ref,
	target *vault.Vault,
	opts PromoteOptions,
) (PromotePlan, error) {
	plan := PromotePlan{
		DryRun: true, Vault: ref.Vault.Name, Path: ref.Path,
		SharedVault: target.Name, Target: target.Name + ":" + ref.Path,
	}
	fm, body, gates, err := noteGates(ref, target, opts.Force)
	if err != nil {
		return plan, err
	}
	if gates[0].Result == GateRefused {
		for _, name := range []string{
			GateStatus, GateRepo, GateDuplicate, GateSecrets, GateClassification, GateCheck,
		} {
			gates = append(gates, skipped(name, "the target is not a vault brain promotes into"))
		}
		return plan.settle(gates), nil
	}
	plan.ID, plan.MOCs = fm.ID, lifecycle.Read(fm).MOCs
	if fm.ID != "" {
		plan.Branch = promoteBranch(ref.Path, fm.ID)
	}
	repo, onto := repoGate(lib, target)
	plan.Base = onto
	copied, warnings, err := mirroredGates(lib, target, ref, fm, body)
	if err != nil {
		return plan, err
	}
	plan.Warnings = warnings
	return plan.settle(append(append(gates, repo), copied...)), nil
}

// settle records the gates and the verdict they give.
func (p PromotePlan) settle(gates []Gate) PromotePlan {
	p.Gates = gates
	p.Verdict = VerdictProceed
	if refusal(gates) != nil {
		p.Verdict = VerdictRefused
	}
	return p
}

// mirroredGates runs copyGates against a mirror of the shared clone in a
// temporary directory, then removes it.
func mirroredGates(
	lib *library.Library,
	shared *vault.Vault,
	ref vault.Ref,
	fm markdown.Frontmatter,
	body []byte,
) (gates []Gate, warnings []string, err error) {
	dir, err := mirror(shared.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("dry run: mirror shared vault %s: %w", shared.Name, err)
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	return copyGates(lib, &vault.Vault{Root: dir, Name: shared.Name}, shared, ref, fm, body)
}

// mirror copies what brain check reads of the vault at root into a new
// temporary directory: every readable markdown note (a linked one as a
// link, which check never reads either), the policy and the secret scan
// config. It leaves out .git.
func mirror(root string) (dir string, err error) {
	dir, err = os.MkdirTemp("", "brain-dry-run-")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(dir))
		}
	}()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return err
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if isNote(rel) {
			return mirrorFile(path, filepath.Join(dir, rel), d, true)
		}
		if slashed := filepath.ToSlash(rel); slashed == check.PolicyFile || slashed == vault.SuspendersFile {
			return mirrorFile(path, filepath.Join(dir, rel), d, false)
		}
		return nil
	})
	return dir, err
}

// isNote reports whether the file at rel is a markdown note.
func isNote(rel string) bool { return strings.HasSuffix(strings.ToLower(rel), ".md") }

// mirrorFile copies the file at src to dst, a symbolic link as a link.
// skipUnreadable passes over a file that cannot be read, the way brain
// check passes over an unreadable note; a config file that cannot be read
// fails the mirror instead.
func mirrorFile(src, dst string, d fs.DirEntry, skipUnreadable bool) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if d.Type()&fs.ModeSymlink != 0 {
		link, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(link, dst)
	}
	if !d.Type().IsRegular() {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		if skipUnreadable {
			return nil
		}
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
