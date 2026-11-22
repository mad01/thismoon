package guard

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// directFrozenVerdicts holds the direct-deny-list verdicts the build that
// added the guard reached for directFrozenCorpus, in the format of
// frozenVerdicts (scriptdeny_diff_test.go). They were recorded once from
// that build. A later build must never record them again: it would write
// its own verdicts, and the test would compare the code with itself. A
// reviewed change goes on directTightened.
const directFrozenVerdicts = "testdata/directdeny_frozen.txt"

// directTightened are hand-reviewed corpus inputs the frozen build allowed
// and the guard now denies.
var directTightened = []string{}

// directFrozenCorpus returns the guard with the tranche deny list, the
// corpus by group, and the temporary directory every corpus directory sits
// in. The groups and the order of their inputs are frozen with the
// verdicts: an input added at the end of a group has no frozen verdict, and
// anywhere else it fails the fingerprint.
func directFrozenCorpus(t *testing.T) (*DirectDenyList, []frozenGroup, string) {
	cwd := t.TempDir()
	g := NewDirectDenyList(config.Config{ClaudeDeny: trancheEntries})
	lines := func(commands []string) []frozenInput {
		out := make([]frozenInput, 0, len(commands))
		for _, c := range commands {
			out = append(out, frozenInput{command: c, cwd: cwd})
		}
		return out
	}
	var table, shapes []string
	for _, b := range directBlocks {
		table = append(table, b.denyCommands()...)
		table = append(table, b.passCommands()...)
	}
	for _, s := range directShapes {
		shapes = append(shapes, s.command)
	}
	return g, []frozenGroup{
		{"tranche", lines(table)},
		{"shapes", lines(shapes)},
		{"generated", lines(directGeneratedCorpus())},
	}, filepath.Dir(cwd)
}

// directGeneratedCorpus crosses each tranche CLI's plain deny form, its
// first two read-only siblings and an echo that names the deny form with
// the shell around a command that generatedCorpus uses: a prefix, an
// operator-line suffix, and a command on the next line.
func directGeneratedCorpus() []string {
	var forms []string
	for _, b := range directBlocks {
		plain := b.tool + " " + b.plain
		forms = append(forms, plain)
		for _, p := range b.passes[:min(2, len(b.passes))] {
			forms = append(forms, b.tool+" "+p)
		}
		forms = append(forms, "echo "+shellQuote(plain))
	}
	var out []string
	for _, f := range forms {
		for _, p := range corpusPrefixes {
			for _, sfx := range corpusSuffixes {
				out = append(out, p+f+sfx)
			}
		}
		for _, tail := range corpusTails {
			out = append(out, f+tail)
		}
	}
	return out
}

// TestDirectDenyListKeepsFrozenVerdicts is the safety net for every
// direct-deny-list change: across the corpus, the guard reaches the verdict
// the frozen build reached (directFrozenVerdicts), except for the inputs on
// directTightened, which it denies now. The table's pass rows stay allowed.
func TestDirectDenyListKeepsFrozenVerdicts(t *testing.T) {
	g, groups, root := directFrozenCorpus(t)
	frozen := readFrozenVerdicts(t, directFrozenVerdicts)
	if got := frozenPrint([]string{strings.Join(uniquePatterns(trancheEntries), "\n")}); got != frozen.patterns {
		t.Fatalf("the tranche deny list changed since the verdicts were frozen (fingerprint %s)", got)
	}
	passes := map[string]bool{}
	for _, b := range directBlocks {
		for _, c := range b.passCommands() {
			passes[c] = true
		}
	}
	tightened := map[string]bool{}
	for _, c := range directTightened {
		tightened[c] = true
	}
	var inputs, frozenDenies int
	for _, group := range groups {
		want, ok := frozen.groups[group.name]
		if !ok {
			t.Fatalf("group %s has no frozen verdicts", group.name)
		}
		if len(group.inputs) < len(want) {
			t.Fatalf("group %s has %d inputs, the frozen verdicts %d", group.name, len(group.inputs), len(want))
		}
		if got := groupPrint(group.inputs[:len(want)], root); got != frozen.prints[group.name] {
			t.Fatalf("group %s changed since the verdicts were frozen (fingerprint %s); "+
				"add new inputs at the end of a group", group.name, got)
		}
		for i, e := range group.inputs {
			now := g.Check(Input{Event: EventBash, Command: e.command, Cwd: e.cwd}) != nil
			inputs++
			checkFrozen(t, e, now, i < len(want) && want[i], i < len(want), passes, tightened)
			if i < len(want) && want[i] {
				frozenDenies++
			}
		}
	}
	t.Logf("corpus %d inputs, the frozen build denies %d", inputs, frozenDenies)
}
