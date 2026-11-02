package cli

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/llm"
)

const (
	// mcpWrapperPath is the launcher an MCP host starts `humanizer mcp`
	// through, in the thismoon checkout four levels above this package.
	mcpWrapperPath = "../../../../recipes/humanizer/humanizer-mcp-sandbox.sh"
	// judgeModelEnv is the model override internal/judge reads on top of
	// kit/llm's provider variables.
	judgeModelEnv = "HUMANIZER_MODEL"
)

// judgeVarsLine matches the wrapper's one list of the variables it pulls
// from the secrets file and lets into the sandbox.
var judgeVarsLine = regexp.MustCompile(`(?m)^judge_vars="([A-Z_ ]+)"$`)

// TestMCPWrapperPullsJudgeEnv keeps recipes/humanizer/humanizer-mcp-sandbox.sh
// in step with kit/llm: the wrapper must pull exactly the variables kit/llm
// reads plus the judge's model override, or humanizer_judge from an MCP client
// finds no backend that a shell finds. Both the secrets pull and the sandbox
// env allow-list must take that one list.
func TestMCPWrapperPullsJudgeEnv(t *testing.T) {
	raw, err := os.ReadFile(mcpWrapperPath)
	if err != nil {
		t.Fatalf("read wrapper: %v", err)
	}
	script := string(raw)

	m := judgeVarsLine.FindStringSubmatch(script)
	if m == nil {
		t.Fatal(`humanizer-mcp-sandbox.sh has no judge_vars="..." line naming the variables`)
	}
	got := strings.Fields(m[1])
	want := append(slices.Clone(llm.ProviderEnv), judgeModelEnv)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("judge_vars is %v, want llm.ProviderEnv plus %s: %v", got, judgeModelEnv, want)
	}

	for _, use := range []string{
		"export_provider_env $judge_vars", // the secrets pull
		"for _v in $judge_vars; do",       // the env -i allow-list
	} {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(use) + `$`).MatchString(script) {
			t.Errorf("humanizer-mcp-sandbox.sh has no %q line", use)
		}
	}
}
