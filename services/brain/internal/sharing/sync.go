package sharing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/ghcli"
	"github.com/mad01/thismoon/services/brain/internal/gitvault"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// maxDirtyNamed caps the files a dirty vault's refusal names.
const maxDirtyNamed = 3

// Sync states of one vault.
const (
	// StateSynced is a vault pulled, and pushed when private and ahead.
	StateSynced = "synced"
	// StateLocalOnly is a git vault with no remote: nothing to pull or push.
	StateLocalOnly = "local_only"
	// StateNotGit is a vault that is not a git repo.
	StateNotGit = "not_git"
	// StateRefused is a vault brain left alone: dirty, diverged, or with
	// no upstream branch. brain never stashes, resets or merges.
	StateRefused = "refused"
	// StateFailed is a vault whose fetch, pull or push failed.
	StateFailed = "failed"
)

// What sync did with a promoting note.
const (
	// ActionRemoved is a merged pull request: the private copy is gone.
	ActionRemoved = "removed"
	// ActionValidated is a closed pull request: the note is validated again.
	ActionValidated = "validated"
	// ActionWaiting is an open pull request, or a merge the shared vault
	// has not been pulled for yet.
	ActionWaiting = "waiting"
	// ActionFailed is a pull request gh could not report on.
	ActionFailed = "failed"
)

// SyncReport is the JSON result of brain sync.
type SyncReport struct {
	// OK is false when a vault was refused or failed, or a promotion could
	// not be settled.
	OK     bool        `json:"ok"`
	Vaults []VaultSync `json:"vaults"`
}

// VaultSync is what sync did in one vault.
type VaultSync struct {
	Vault   string `json:"vault"`
	Shared  bool   `json:"shared"`
	State   string `json:"state"`
	Message string `json:"message"`
	Pulled  int    `json:"pulled"`
	Pushed  int    `json:"pushed"`
	// Promotions are the vault's promoting notes and what became of each.
	Promotions []Settled `json:"promotions,omitempty"`

	repo           *gitvault.Repo
	remote, branch string
}

// Settled is one promoting note sync looked at.
type Settled struct {
	Path   string `json:"path"`
	ID     string `json:"id,omitempty"`
	PR     string `json:"pr"`
	State  string `json:"state,omitempty"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

// Sync syncs each vault of set with its remote, then settles the private
// vaults' promotions. It pulls every vault (fast-forward only), asks gh
// about each promoting note's pull request (a merge removes the private
// copy, a close returns it to validated), and last pushes each private
// vault that is ahead, so the commits the settling made go out in the same
// run. A shared vault is never pushed: its notes change through pull
// requests. A vault with no remote is not pulled or pushed; one that is
// dirty, diverged or tracks no remote branch is refused and left as it is.
func Sync(ctx context.Context, lib *library.Library, set *vault.Set, now time.Time) SyncReport {
	report := SyncReport{OK: true}
	vaults := set.Vaults()
	syncs := make([]VaultSync, len(vaults))
	for i, v := range vaults {
		syncs[i] = pull(lib, v)
	}
	for i, v := range vaults {
		if !v.Shared && syncs[i].State != StateRefused && syncs[i].State != StateFailed {
			syncs[i].Promotions = settle(ctx, lib, v, now)
		}
	}
	for i, v := range vaults {
		if !v.Shared && syncs[i].State == StateSynced {
			push(v, &syncs[i])
		}
		report.OK = report.OK && healthy(syncs[i])
	}
	report.Vaults = syncs
	return report
}

func healthy(s VaultSync) bool {
	if s.State == StateRefused || s.State == StateFailed {
		return false
	}
	for _, p := range s.Promotions {
		if p.Action == ActionFailed {
			return false
		}
	}
	return true
}

// pull fetches the vault's upstream and fast-forwards it.
func pull(lib *library.Library, v *vault.Vault) VaultSync {
	s := VaultSync{Vault: v.Name, Shared: v.Shared, repo: lib.Git(v)}
	if !s.repo.IsRepo() {
		return s.with(StateNotGit, "not a git repo: nothing to pull or push")
	}
	remotes, err := s.repo.Remotes()
	if err != nil {
		return s.with(StateFailed, err.Error())
	}
	if len(remotes) == 0 {
		return s.with(StateLocalOnly, "local-only: no remote, so nothing is pulled or pushed")
	}
	if s.remote, s.branch, err = s.repo.Upstream(); err != nil {
		return s.with(StateRefused, fmt.Sprintf(
			"%v; brain syncs the branch with its upstream (set it with: git -C %s branch --set-upstream-to %s/<branch>)",
			err,
			v.Root,
			remotes[0],
		))
	}
	if refusal := dirtyRefusal(s.repo); refusal != "" {
		return s.with(StateRefused, refusal)
	}
	release, err := v.LockWrites()
	if err != nil {
		return s.with(StateFailed, err.Error())
	}
	defer func() { _ = release() }()
	if err := s.repo.Fetch(s.remote); err != nil {
		return s.with(StateFailed, err.Error())
	}
	ahead, behind, err := s.repo.Counts(s.upstream())
	if err != nil {
		return s.with(StateFailed, err.Error())
	}
	if ahead > 0 && behind > 0 {
		return s.with(StateRefused, fmt.Sprintf(
			"diverged from %s: %d local and %d remote commit(s); rebase or merge it by hand, "+
				"brain only fast-forwards", s.short(), ahead, behind))
	}
	if behind > 0 {
		if err := s.repo.PullFFOnly(); err != nil {
			return s.with(StateFailed, err.Error())
		}
		s.Pulled = behind
		return s.with(StateSynced, fmt.Sprintf("pulled %d commit(s) from %s", behind, s.short()))
	}
	msg := "up to date with " + s.short()
	if v.Shared && ahead > 0 {
		msg += fmt.Sprintf("; %d local commit(s) stay unpushed, since shared notes change "+
			"through pull requests", ahead)
	}
	return s.with(StateSynced, msg)
}

// dirtyRefusal is the refusal of a vault with uncommitted changes to
// tracked files, or "" for a clean one.
func dirtyRefusal(repo *gitvault.Repo) string {
	dirty, err := repo.Dirty()
	if err != nil {
		return err.Error()
	}
	if len(dirty) == 0 {
		return ""
	}
	named := dirty[:min(len(dirty), maxDirtyNamed)]
	more := ""
	if extra := len(dirty) - len(named); extra > 0 {
		more = fmt.Sprintf(" and %d more", extra)
	}
	return fmt.Sprintf("uncommitted changes in %s%s; commit or discard them first "+
		"(brain never stashes or resets)", strings.Join(named, ", "), more)
}

// push pushes a private vault that is ahead of its upstream.
func push(v *vault.Vault, s *VaultSync) {
	ahead, _, err := s.repo.Counts(s.upstream())
	if err != nil {
		s.State, s.Message = StateFailed, err.Error()
		return
	}
	if ahead == 0 {
		return
	}
	release, err := v.LockWrites()
	if err != nil {
		s.State, s.Message = StateFailed, err.Error()
		return
	}
	defer func() { _ = release() }()
	if err := s.repo.Push(s.remote, s.branch); err != nil {
		s.State, s.Message = StateFailed, err.Error()
		return
	}
	s.Pushed = ahead
	s.Message += fmt.Sprintf("; pushed %d commit(s)", ahead)
}

// settle asks gh about each promoting note of the vault and acts on the
// answer.
func settle(ctx context.Context, lib *library.Library, v *vault.Vault, now time.Time) []Settled {
	sn, err := vault.NewSet(v).Snapshot()
	if err != nil {
		return []Settled{{Action: ActionFailed, Detail: err.Error()}}
	}
	var out []Settled
	for _, n := range sn.Notes() {
		f := lifecycle.Read(n.Frontmatter)
		if n.Status() != lifecycle.Promoting || f.SharedPR == "" {
			continue
		}
		s := Settled{Path: n.Ref.Path, ID: n.Frontmatter.ID, PR: f.SharedPR}
		status, err := ghcli.ViewPR(ctx, f.SharedPR)
		if err != nil {
			s.Action, s.Detail = ActionFailed, err.Error()
			out = append(out, s)
			continue
		}
		s.State = status.State
		out = append(out, settleOne(lib, n.Ref, s, now))
	}
	return out
}

// settleOne acts on one pull request's state.
func settleOne(lib *library.Library, ref vault.Ref, s Settled, now time.Time) Settled {
	switch s.State {
	case ghcli.StateMerged:
		shared, ok := sharedCopy(lib, s.ID)
		if !ok {
			s.Action = ActionWaiting
			s.Detail = "merged, but no shared vault here holds id " + s.ID +
				" yet; sync the shared vault too, then this one"
			return s
		}
		if err := ref.Vault.RemovePromoted(ref.Abs(), s.PR); err != nil {
			s.Action, s.Detail = ActionFailed, err.Error()
			return s
		}
		s.Action = ActionRemoved
		s.Detail = "merged: removed the private copy; links resolve to " + shared.String()
	case ghcli.StateClosed:
		if _, err := ref.Vault.EndPromotion(ref.Abs(), s.PR, now); err != nil {
			s.Action, s.Detail = ActionFailed, err.Error()
			return s
		}
		s.Action, s.Detail = ActionValidated, "closed without a merge: validated again, shared_pr removed"
	default:
		s.Action, s.Detail = ActionWaiting, "pull request is "+strings.ToLower(s.State)
	}
	return s
}

// sharedCopy finds the note with id in the configured shared vaults.
func sharedCopy(lib *library.Library, id string) (vault.Ref, bool) {
	if id == "" {
		return vault.Ref{}, false
	}
	var shared []*vault.Vault
	for _, v := range lib.Set().Vaults() {
		if v.Shared {
			shared = append(shared, v)
		}
	}
	ref, err := vault.NewSet(shared...).Locate("id:" + id)
	return ref, err == nil
}

func (s VaultSync) with(state, message string) VaultSync {
	s.State, s.Message = state, message
	return s
}

// upstream is the remote-tracking ref of the branch's upstream.
func (s VaultSync) upstream() string {
	return "refs/remotes/" + s.remote + "/" + s.branch
}

func (s VaultSync) short() string { return s.remote + "/" + s.branch }

// errSyncFailed is the error brain sync exits with when the report is not
// ok.
var errSyncFailed = errors.New("sync left a vault unsynced or a promotion unsettled")

// Err returns nil for an ok report, else an error naming what went wrong.
func (r SyncReport) Err() error {
	if r.OK {
		return nil
	}
	var names []string
	for _, s := range r.Vaults {
		if !healthy(s) {
			names = append(names, s.Vault)
		}
	}
	return fmt.Errorf("%w: %s", errSyncFailed, strings.Join(names, ", "))
}
