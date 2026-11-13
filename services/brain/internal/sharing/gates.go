package sharing

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/check"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// The gates of promote --to, in the order it runs them. Promote stops at
// the first refusal; DryRun runs every gate it can and reports each.
const (
	// GateTarget: the target is a shared vault and the note is not in one.
	GateTarget = "target"
	// GateStatus: the note is validated, has an id, and is not promoting
	// through an open pull request (unless forced).
	GateStatus = "status"
	// GateRepo: the shared vault is a git clone with a remote to open the
	// pull request on.
	GateRepo = "repo"
	// GateDuplicate: the shared vault has no note at the path yet.
	GateDuplicate = "duplicate"
	// GateSecrets: the shared copy holds no secret and no NUL byte.
	GateSecrets = "secrets"
	// GateClassification: the shared vault's policy accepts the note's
	// classification.
	GateClassification = "classification"
	// GateCheck: brain check finds no error in the shared copy under the
	// shared vault's policy.
	GateCheck = "check"
)

// Gate results.
const (
	GatePass    = "pass"
	GateRefused = "refused"
	// GateSkipped marks a gate an earlier refusal left nothing to check.
	GateSkipped = "skipped"
)

// Gate is one check promote --to makes before it pushes anything.
type Gate struct {
	Name   string `json:"name"`
	Result string `json:"result"`
	// Detail says what passed, names the rule that refused, or says why
	// the gate was skipped.
	Detail string `json:"detail"`
	err    error
}

func passed(name, detail string) Gate { return Gate{Name: name, Result: GatePass, Detail: detail} }

func skipped(name, why string) Gate { return Gate{Name: name, Result: GateSkipped, Detail: why} }

// gateOf is the gate that passed with detail when err is nil, else the one
// that refused with err.
func gateOf(name, detail string, err error) Gate {
	if err != nil {
		return Gate{Name: name, Result: GateRefused, Detail: err.Error(), err: err}
	}
	return passed(name, detail)
}

// refusal returns the error of the first refused gate, nil when none was.
func refusal(gates []Gate) error {
	for _, g := range gates {
		if g.Result == GateRefused {
			return g.err
		}
	}
	return nil
}

// details lists what each passed gate checked: the checks a pull request
// body names.
func details(gates []Gate) []string {
	var out []string
	for _, g := range gates {
		if g.Result == GatePass {
			out = append(out, g.Detail)
		}
	}
	return out
}

// noteGates runs the gates on the private note alone: target and status.
// When the target gate refuses, the note is not read and only that gate
// is returned. An error is a note that cannot be read, not a refusal.
func noteGates(
	ref vault.Ref,
	target *vault.Vault,
	force bool,
) (markdown.Frontmatter, []byte, []Gate, error) {
	if err := promotable(ref, target); err != nil {
		return markdown.Frontmatter{}, nil, []Gate{gateOf(GateTarget, "", err)}, nil
	}
	gates := []Gate{passed(GateTarget, target.Name+" is a shared vault")}
	fm, body, err := ref.Vault.ReadFrontmatter(ref.Abs())
	if err != nil {
		return markdown.Frontmatter{}, nil, nil, err
	}
	detail := fmt.Sprintf("%s is %s, with id %s", ref.Path, vault.StatusOf(ref.Path, fm), fm.ID)
	return fm, body, append(gates, gateOf(GateStatus, detail, ready(ref, fm, force))), nil
}

// repoGate is the gate of the shared vault's clone: what prTarget finds.
// It returns the remote branch the pull request goes to, "" on a refusal.
func repoGate(lib *library.Library, shared *vault.Vault) (Gate, string) {
	_, remote, base, err := prTarget(lib, shared)
	if err != nil {
		return gateOf(GateRepo, "", err), ""
	}
	return repoPassed(shared, remote, base), remote + "/" + base
}

// repoPassed is the repo gate of a clone whose pull requests go to base on
// remote.
func repoPassed(shared *vault.Vault, remote, base string) Gate {
	return passed(GateRepo, fmt.Sprintf("the pull request goes to %s/%s of %s's clone",
		remote, base, shared.Name))
}

// copyGates runs the gates on the shared copy of the note against tree, a
// checkout of the shared vault brain may write to: the worktree of a real
// promote, the mirror of a dry run. Placing the copy in tree is the secret
// scan, the one every write runs; brain check then reads it there under
// the policy tree holds. It returns the gates, and the warnings: check
// findings below error level, and a classification the policy only warns
// about. An error is a failure to check, not a refusal.
func copyGates(
	lib *library.Library,
	tree, shared *vault.Vault,
	ref vault.Ref,
	fm markdown.Frontmatter,
	body []byte,
) ([]Gate, []string, error) {
	var gates []Gate
	taken := duplicateGate(tree, shared, ref.Path)
	gates = append(gates, taken)
	secrets, err := placeCopy(tree, ref.Path, fm, body, taken.Result != GatePass)
	if err != nil {
		return nil, nil, err
	}
	gates = append(gates, secrets)
	// A policy that does not load is the check gate's refusal (rule
	// policy); the classification gate then reads the lenient default, as
	// brain check does.
	policy, _ := check.LoadPolicy(tree.Root)
	classification, warnings := classificationGate(
		policy,
		shared.Name,
		lifecycle.Read(fm).Classification,
	)
	gates = append(gates, classification)
	if secrets.Result != GatePass {
		return append(
				gates,
				skipped(GateCheck, "the copy was not placed, so brain check had nothing to read"),
			),
			warnings, nil
	}
	checked, checkWarnings, err := checkGate(lib, tree, shared, ref.Path)
	if err != nil {
		return nil, nil, err
	}
	return append(gates, checked), append(warnings, checkWarnings...), nil
}

// duplicateGate refuses a path the shared vault has already.
func duplicateGate(tree, shared *vault.Vault, rel string) Gate {
	if _, err := os.Stat(filepath.Join(tree.Root, filepath.FromSlash(rel))); err == nil {
		return gateOf(GateDuplicate, "", fmt.Errorf("%w: shared vault %s has %s already",
			vault.ErrExists, shared.Name, rel))
	}
	return passed(GateDuplicate, shared.Name+" has no "+rel+" yet")
}

// placeCopy writes the shared copy of the note into tree at rel: the
// note without its private-only fields, validated. The write refuses a
// secret or a NUL byte before it touches disk, which is the secrets gate.
// skip leaves the copy out, when the path is taken.
func placeCopy(
	tree *vault.Vault,
	rel string,
	fm markdown.Frontmatter,
	body []byte,
	skip bool,
) (Gate, error) {
	if skip {
		return skipped(
			GateSecrets,
			"the path is taken, so the copy was not placed and scanned",
		), nil
	}
	copyFM := lifecycle.ForShared(fm)
	copyFM.Status = lifecycle.Validated
	_, err := tree.CreateAt(rel, copyFM, body)
	switch {
	case err == nil:
		return passed(GateSecrets, "secret scan: nothing found"), nil
	case errors.Is(err, vault.ErrSecretFound), errors.Is(err, vault.ErrInvalidContent),
		errors.Is(err, vault.ErrExists):
		return gateOf(GateSecrets, "", err), nil
	}
	return Gate{}, err
}

// classificationGate checks the note's classification against the shared
// vault's policy, at the level the policy gives the classification rule.
// A note without a known classification passes here: brain check's
// missing-field and schema rules report it.
func classificationGate(p check.Policy, shared, classification string) (Gate, []string) {
	accepts := strings.Join(accepted(p), ", ")
	if !slices.Contains(lifecycle.Classifications, classification) {
		return passed(GateClassification, "the note has no known classification; "+
			"brain check's missing-field and schema rules cover it"), nil
	}
	if p.Allows(classification) {
		return passed(
			GateClassification,
			fmt.Sprintf("classification %s is accepted by %s (accepts: %s)",
				classification, shared, accepts),
		), nil
	}
	msg := fmt.Sprintf("classification %s is not accepted by %s (accepts: %s)",
		classification, shared, accepts)
	switch p.Level(check.RuleClassification) {
	case check.Off:
		return passed(
			GateClassification,
			msg+", but its policy turns the classification rule off",
		), nil
	case check.Warning:
		return passed(GateClassification, msg+", which its policy only warns about"),
			[]string{check.RuleClassification + ": " + msg}
	}
	return gateOf(GateClassification, "", errors.New(msg)), nil
}

// accepted lists the classifications a vault with policy p accepts, every
// one when the policy names none.
func accepted(p check.Policy) []string {
	if len(p.Classifications) == 0 {
		return slices.Clone(lifecycle.Classifications)
	}
	return slices.Clone(p.Classifications)
}

// checkGate runs brain check on the copy at rel in tree. Classification
// findings are left to the classification gate.
func checkGate(
	lib *library.Library,
	tree, shared *vault.Vault,
	rel string,
) (Gate, []string, error) {
	report, err := checkCopy(lib, tree, shared, rel)
	if err != nil {
		return Gate{}, nil, err
	}
	errs := findings(report, check.Error)
	warnings := findings(report, check.Warning)
	if len(errs) > 0 {
		if len(errs) > maxReportedFindings {
			errs = append(errs[:maxReportedFindings],
				fmt.Sprintf("and %d more", len(errs)-maxReportedFindings))
		}
		return gateOf(GateCheck, "", fmt.Errorf("brain check against %s's policy: %s",
			shared.Name, strings.Join(errs, "; "))), warnings, nil
	}
	return passed(
		GateCheck,
		fmt.Sprintf("brain check against %s's policy: no errors, %d warning(s)",
			shared.Name, len(warnings)),
	), warnings, nil
}

// checkCopy runs brain check on the copy at rel in tree, tree standing in
// for the shared vault: its policy, its secret scan config, and links
// resolving among the shared vaults only.
func checkCopy(lib *library.Library, tree, shared *vault.Vault, rel string) (check.Report, error) {
	stand := *tree
	stand.Shared = true
	var vaults []*vault.Vault
	for _, v := range lib.Set().Vaults() {
		switch {
		case v == shared:
			vaults = append(vaults, &stand)
		case v.Shared:
			vaults = append(vaults, v)
		}
	}
	return check.Run(check.Options{
		Set:     vault.NewSet(vaults...),
		Targets: []vault.Ref{{Vault: &stand, Path: rel}},
	})
}

// findings lists the report's findings at level as "rule: message",
// leaving out the classification rule, which has a gate of its own.
func findings(r check.Report, level string) []string {
	var out []string
	for _, f := range r.Findings {
		if f.Level == level && f.Rule != check.RuleClassification {
			out = append(out, f.Rule+": "+f.Message)
		}
	}
	return out
}
