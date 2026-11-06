package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/check"
	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/gitvault"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Git states brain vaults reports.
const (
	gitRemote    = "remote"
	gitLocalOnly = "local-only"
	gitNone      = "none"
)

// vaultsReport is the JSON result of brain vaults.
type vaultsReport struct {
	Vaults []vaultInfo `json:"vaults"`
}

// vaultInfo is one configured vault as brain vaults describes it.
type vaultInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Shared  bool   `json:"shared"`
	Default bool   `json:"default"`
	// Git is remote, local-only (a repo with no origin) or none (the
	// directory is not the top of a git repo); Remote is origin's URL.
	Git    string `json:"git"`
	Remote string `json:"remote,omitempty"`
	// AcceptsAny is set when the vault takes every classification: no
	// policy, or one naming none. Accepts lists them otherwise.
	AcceptsAny bool        `json:"accepts_any"`
	Accepts    []string    `json:"accepts"`
	Policy     *vaultRules `json:"policy,omitempty"`
	// Notes counts the notes in the vault's tiers.
	Notes int `json:"notes"`
	// Error says why the vault could not be read; the other fields are
	// then what the config says.
	Error string `json:"error,omitempty"`
}

// vaultRules is a vault's .brain/policy.yaml as written.
type vaultRules struct {
	File            string            `json:"file"`
	Classifications []string          `json:"classifications"`
	Rules           map[string]string `json:"rules"`
	// Error says why the policy cannot be used; brain check then refuses
	// every note of the vault until it is fixed.
	Error string `json:"error,omitempty"`
}

func newVaultsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "vaults",
		Short: "List the configured vaults: shared or private, git state, what each accepts",
		Long: `List every configured vault, in the order brain resolves a bare note path,
one line each: its name and path, whether it is shared (read-only here) and
whether it is the default write target, its git state (remote and origin's
URL, local-only for a repo with no origin, none when the directory is not
the top of a git repo), the classifications its .brain/policy.yaml accepts
("any" without a policy or with an empty list) and how many notes its tiers
hold. A vault with a policy gets an indented line with the rule levels the
policy sets.

Run it before brain promote --to to see what a shared vault accepts, then
check the promote with promote --to <vault> --dry-run. It reads the vaults
and their git config alone: no network, no writes. A vault whose directory
is missing or unreadable is listed with an error and does not fail the
command. --vault narrows the list. With -o json, prints {"vaults": [{"name",
"path", "shared", "default", "git", "remote", "accepts_any", "accepts",
"policy" {"file", "classifications", "rules", "error"}, "notes",
"error"}]}.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			report, err := describeVaults(cfg, a.vaults)
			if err != nil {
				return err
			}
			return a.emit(cmd.OutOrStdout(), report, func(w io.Writer) error {
				writeVaultsText(w, report)
				return nil
			})
		},
	}
}

// describeVaults describes the configured vaults the names select, every
// one when names is empty, in config order. It opens no vault through the
// library, so a vault whose directory is gone is described too.
func describeVaults(cfg *config.Config, names []string) (vaultsReport, error) {
	configured := make([]*vault.Vault, 0, len(cfg.Vaults))
	for _, c := range cfg.Vaults {
		configured = append(configured, &vault.Vault{
			Root: c.Path, Name: c.Name, Shared: c.Shared, Default: c.Default,
		})
	}
	set, err := vault.NewSet(configured...).Filter(names)
	if err != nil {
		return vaultsReport{}, err
	}
	report := vaultsReport{Vaults: make([]vaultInfo, 0, len(set.Vaults()))}
	for _, v := range set.Vaults() {
		report.Vaults = append(report.Vaults, describeVault(v))
	}
	return report, nil
}

// describeVault reads one vault's git state, policy and note count.
func describeVault(v *vault.Vault) vaultInfo {
	info := vaultInfo{
		Name: v.Name, Path: v.Root, Shared: v.Shared, Default: v.Default,
		Git: gitNone, AcceptsAny: true, Accepts: []string{},
	}
	if err := readableDir(v.Root); err != nil {
		info.Error = err.Error()
		return info
	}
	info.Git, info.Remote = gitState(v.Root)
	info.Policy = policyOf(v.Root)
	if info.Policy != nil && info.Policy.Error == "" && len(info.Policy.Classifications) > 0 {
		info.AcceptsAny, info.Accepts = false, info.Policy.Classifications
	}
	n, err := countNotes(v)
	if err != nil {
		info.Error = err.Error()
	}
	info.Notes = n
	return info
}

// readableDir says why root is not a directory brain can list.
func readableDir(root string) error {
	st, err := os.Stat(root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errors.New("the directory does not exist")
	case err != nil:
		return err
	case !st.IsDir():
		return errors.New("the path is not a directory")
	}
	if _, err := os.ReadDir(root); err != nil {
		return fmt.Errorf("the directory cannot be read: %w", err)
	}
	return nil
}

// gitState is the vault's git state and, for a remote, origin's URL with
// any credentials masked.
func gitState(root string) (string, string) {
	repo := gitvault.New(root)
	if !repo.IsRepo() {
		return gitNone, ""
	}
	remotes, err := repo.Remotes()
	if err != nil || !slices.Contains(remotes, "origin") {
		return gitLocalOnly, ""
	}
	u, err := repo.RemoteURL("origin")
	if err != nil {
		return gitRemote, ""
	}
	return gitRemote, maskCredentials(u)
}

// maskCredentials hides the user info of an http(s) remote URL, where a
// token often sits, and the password of any other URL. A URL that does not
// parse, such as git@host:owner/repo, is returned as it is.
func maskCredentials(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if u.Scheme == "http" || u.Scheme == "https" {
		u.User = url.User("xxxxx")
		return u.String()
	}
	return u.Redacted()
}

// policyOf reads the vault's policy file, nil when it has none.
func policyOf(root string) *vaultRules {
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(check.PolicyFile))); err != nil {
		return nil
	}
	rules := &vaultRules{File: check.PolicyFile, Classifications: []string{}, Rules: map[string]string{}}
	p, err := check.LoadPolicy(root)
	if err != nil {
		rules.Error = err.Error()
		return rules
	}
	if len(p.Classifications) > 0 {
		rules.Classifications = slices.Clone(p.Classifications)
	}
	for rule, level := range p.Rules {
		rules.Rules[rule] = level
	}
	return rules
}

// countNotes counts the notes in the vault's tiers.
func countNotes(v *vault.Vault) (int, error) {
	sn, err := vault.NewSet(v).Snapshot()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, note := range sn.Notes() {
		if note.Tier() != "" {
			n++
		}
	}
	return n, nil
}

func writeVaultsText(w io.Writer, r vaultsReport) {
	for _, v := range r.Vaults {
		fmt.Fprintf(w, "%s: %s (%s)", v.Name, v.Path, vaultRole(v))
		if v.Error != "" {
			fmt.Fprintf(w, ", error: %s\n", v.Error)
			continue
		}
		git := "git " + v.Git
		if v.Remote != "" {
			git += " " + v.Remote
		}
		accepts := "any"
		if !v.AcceptsAny {
			accepts = strings.Join(v.Accepts, ", ")
		}
		fmt.Fprintf(w, ", %s, %d note(s), accepts %s\n", git, v.Notes, accepts)
		if v.Policy != nil {
			fmt.Fprintf(w, "  policy %s: %s\n", v.Policy.File, policyLine(v.Policy))
		}
	}
}

// vaultRole says whether the vault is shared or private, and the default.
func vaultRole(v vaultInfo) string {
	switch {
	case v.Shared:
		return "shared, read-only: notes reach it through promote --to"
	case v.Default:
		return "private, default write target"
	}
	return "private"
}

// policyLine is the rule levels a policy sets, one line; the vault's own
// line already says what it accepts.
func policyLine(p *vaultRules) string {
	if p.Error != "" {
		return "unusable: " + p.Error + "; brain check refuses every note until it is fixed"
	}
	if len(p.Rules) == 0 {
		return "every rule at its default level"
	}
	rules := make([]string, 0, len(p.Rules))
	for rule, level := range p.Rules {
		rules = append(rules, rule+"="+level)
	}
	sort.Strings(rules)
	return "rules " + strings.Join(rules, ", ")
}
