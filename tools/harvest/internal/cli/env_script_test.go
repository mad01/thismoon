package cli

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/mad01/thismoon/kit/llm"
)

// The recipe files, in the thismoon checkout four levels above this package.
const (
	envScriptPath = "../../../../recipes/harvest/harvest-env.sh"
	recipePath    = "../../../../recipes/harvest/recipe.toml"
)

// envCall matches the line in harvest-env.sh that hands the shared helper
// (recipes/lib/provider-env.sh) the variables to pull from the secrets file.
var envCall = regexp.MustCompile(`(?m)^export_provider_env ([A-Z_ ]+)$`)

// TestEnvScriptPullsProviderEnv keeps recipes/harvest/harvest-env.sh, the
// launcher the SessionEnd hook runs, in step with kit/llm: the script must
// pull exactly the variables kit/llm reads, or the model step finds no
// backend that a shell finds.
func TestEnvScriptPullsProviderEnv(t *testing.T) {
	raw, err := os.ReadFile(envScriptPath)
	if err != nil {
		t.Fatalf("read the launcher: %v", err)
	}
	m := envCall.FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatal("harvest-env.sh has no `export_provider_env NAME...` line naming the variables")
	}
	got := strings.Fields(m[1])
	want := slices.Clone(llm.ProviderEnv)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("harvest-env.sh pulls %v, want llm.ProviderEnv %v", got, want)
	}
}

// TestRecipeKeepsEnvScriptExecutable checks the recipe's chmod item names
// the script, since the hook execs it by path.
func TestRecipeKeepsEnvScriptExecutable(t *testing.T) {
	raw, err := os.ReadFile(recipePath)
	if err != nil {
		t.Fatalf("read recipe.toml: %v", err)
	}
	var recipe struct {
		Hooks struct {
			Builds map[string]struct {
				Commands []string `toml:"commands"`
			} `toml:"builds"`
		} `toml:"hooks"`
	}
	if _, err := toml.Decode(string(raw), &recipe); err != nil {
		t.Fatalf("decode recipe.toml: %v", err)
	}
	item, ok := recipe.Hooks.Builds["harvest_chmod_scripts"]
	if !ok {
		t.Fatal("recipe.toml has no [hooks.builds.harvest_chmod_scripts]")
	}
	if !strings.Contains(strings.Join(item.Commands, "\n"), "chmod +x harvest-env.sh") {
		t.Errorf("harvest_chmod_scripts runs %q, want it to chmod +x harvest-env.sh", item.Commands)
	}
}
