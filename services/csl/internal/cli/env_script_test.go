package cli

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/mad01/thismoon/kit/llm"
)

// envLoop matches the loop in csl-env.sh that names the variables it pulls
// from the secrets file.
var envLoop = regexp.MustCompile(`(?m)^for _v in ([A-Z_ ]+); do$`)

// TestEnvScriptPullsProviderEnv keeps recipes/csl/csl-env.sh, the launcher an
// MCP host starts csl through, in step with kit/llm: the script must pull
// exactly the variables kit/llm reads, or a csl_ask call from an MCP client
// finds no backend that a shell finds.
func TestEnvScriptPullsProviderEnv(t *testing.T) {
	script := readRepoFile(t, filepath.Join("recipes", "csl", "csl-env.sh"))
	m := envLoop.FindStringSubmatch(script)
	if m == nil {
		t.Fatal("csl-env.sh has no `for _v in ...; do` loop naming the variables")
	}
	got := strings.Fields(m[1])
	want := slices.Clone(llm.ProviderEnv)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("csl-env.sh pulls %v, want llm.ProviderEnv %v", got, want)
	}
}

// TestRecipeKeepsEnvScriptExecutable checks the recipe's chmod item names
// the script, since the MCP host execs it by path.
func TestRecipeKeepsEnvScriptExecutable(t *testing.T) {
	var recipe struct {
		Hooks struct {
			Builds map[string]struct {
				Commands []string `toml:"commands"`
			} `toml:"builds"`
		} `toml:"hooks"`
	}
	if _, err := toml.Decode(
		readRepoFile(t, filepath.Join("recipes", "csl", "recipe.toml")),
		&recipe,
	); err != nil {
		t.Fatalf("decode recipe.toml: %v", err)
	}
	item, ok := recipe.Hooks.Builds["csl_chmod_scripts"]
	if !ok {
		t.Fatal("recipe.toml has no [hooks.builds.csl_chmod_scripts]")
	}
	if !strings.Contains(strings.Join(item.Commands, "\n"), "chmod +x csl-env.sh") {
		t.Errorf("csl_chmod_scripts runs %q, want it to chmod +x csl-env.sh", item.Commands)
	}
}
