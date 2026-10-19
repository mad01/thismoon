package eval

import (
	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/checkout"
	"github.com/mad01/thismoon/tools/factory-eval/internal/claude"
)

// Arm names.
const (
	ArmSingle  = "single"
	ArmFactory = "factory"
)

// mcpConfig gives arm sessions one MCP server, humanizer. With
// --strict-mcp-config no GitHub, Linear, present or other outward-facing
// server exists inside a run.
const mcpConfig = `{"mcpServers":{"humanizer":{"type":"stdio","command":"humanizer","args":["mcp"]}}}`

// gitSubcommands are the git commands an arm may run, plain and with -C:
// enough to branch, claim a worktree, commit and inspect, nothing that
// reaches a remote or rewrites config.
var gitSubcommands = []string{
	"add", "commit", "switch", "checkout", "branch", "worktree", "diff", "log", "status",
	"show", "rev-parse", "ls-files", "merge-base",
}

// denied is the deny list both arms share. Deny rules beat the allow
// rules the user's own settings add, so `make install` and `go install`
// can never overwrite installed binaries from inside a run, and no git
// command reaches a remote even where the user's settings allow fetch or
// pull: that matters because git may run outside the sandbox (see
// userExclusions), with the network open. `git * --output*` closes the
// --output flag of diff, log and show, which writes anywhere.
var denied = []string{
	"Bash(git push*)", "Bash(git -C * push*)", "Bash(git fetch*)", "Bash(git -C * fetch*)",
	"Bash(git pull*)", "Bash(git -C * pull*)", "Bash(git clone*)", "Bash(git -C * clone*)",
	"Bash(git ls-remote*)", "Bash(git -C * ls-remote*)", "Bash(git submodule*)",
	"Bash(git -C * submodule*)", "Bash(git remote*)", "Bash(git -C * remote*)",
	"Bash(git config*)", "Bash(git -C * config*)", "Bash(git * --output*)",
	"Bash(gh *)", "Bash(ralph *)",
	"Bash(make install*)", "Bash(make * install*)", "Bash(go install *)", "WebFetch", "WebSearch",
}

// bashTools are the Bash allow rules both arms share: make and go in the
// clone, humanizer, and git plain and with -C.
func bashTools() []string {
	tools := []string{
		"Bash(make -C * build*)", "Bash(make -C * test*)", "Bash(make build*)", "Bash(make test*)",
		"Bash(go build *)", "Bash(go test *)", "Bash(go vet *)", "Bash(humanizer *)",
	}
	for _, sub := range gitSubcommands {
		tools = append(tools, "Bash(git "+sub+" *)", "Bash(git -C * "+sub+" *)")
	}
	return tools
}

// allowedTools is the arm's allow list. Both arms get the same file, git,
// build and humanizer tools; only the factory arm may spawn workers and
// message them.
func allowedTools(arm string) []string {
	tools := []string{"Read", "Edit", "Write", "Glob", "Grep", "Skill", "mcp__humanizer__*"}
	tools = append(tools, bashTools()...)
	if arm == ArmFactory {
		tools = append(tools, "Agent", "SendMessage")
	}
	return tools
}

func disallowedTools(arm string) []string {
	tools := append([]string{}, denied...)
	if arm == ArmSingle {
		tools = append(tools, "Agent", "SendMessage")
	}
	return tools
}

// armPermissions is the permissions block of an arm's settings file: the
// arm's allow list, and its deny list followed by a deny rule per user
// sandbox exclusion the arms do not need (see userExclusions) and the
// isolation rules. The lists also stay on the command line. The file
// carries them because Claude Code documents settings-file rules as
// covering the whole session, subagents included, and never says that of
// --allowedTools. Under dontAsk a factory worker with no allow rule would
// be denied every edit and command, and the factory arm would fail for a
// reason unrelated to the factory.
func armPermissions(arm string, iso Isolation, excluded []string) Permissions {
	exclusionDenies, _ := userExclusions(excluded, bashTools())
	deny := append(disallowedTools(arm), exclusionDenies...)
	return Permissions{
		Allow: allowedTools(arm),
		Deny:  append(deny, iso.DenyRules()...),
	}
}

// settingsPermissions maps each arm to the permissions block its runs'
// settings files hold.
func settingsPermissions(arms []string, iso Isolation, excluded []string) map[string]Permissions {
	perms := make(map[string]Permissions, len(arms))
	for _, arm := range arms {
		perms[arm] = armPermissions(arm, iso, excluded)
	}
	return perms
}

// armArgs is the full argv for one arm session at one tier, in one
// checkout, with the harness settings file at settings.
func armArgs(arm string, tier cases.Tier, co *checkout.Checkout, settings string) []string {
	return claude.ArmArgs(claude.ArmFlags{
		Model:        tier.Model,
		Effort:       tier.Effort,
		BudgetUSD:    tier.BudgetUSD,
		AddDir:       co.Root,
		ReportSchema: ReportSchema,
		MCPConfig:    mcpConfig,
		Allowed:      allowedTools(arm),
		Disallowed:   disallowedTools(arm),
		Settings:     settings,
	})
}
