package main

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// sharedCaseRun is a made-up outcome of a shared-vault case and the types
// of the graders it should fail, llm left out.
type sharedCaseRun struct {
	name    string
	ev      Evidence
	failing []string
}

// TestSharedVaultCaseGraders grades the three shared-vault cases against
// made-up outcomes: the right one passes every grader but the judge, and
// each wrong one fails the graders written to catch it.
func TestSharedVaultCaseGraders(t *testing.T) {
	cases, err := loadCases("cases")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Case{}
	for _, c := range cases {
		byID[c.ID] = c
	}
	runs := map[string][]sharedCaseRun{
		"brain-shared-readonly-edit":   readonlyEditRuns(),
		"brain-shared-private-search":  privateSearchRuns(),
		"brain-promote-to-shared-gate": promoteGateRuns(t),
	}
	for id, tests := range runs {
		c, ok := byID[id]
		if !ok {
			t.Fatalf("no case %s", id)
		}
		for _, tc := range tests {
			t.Run(id+"/"+tc.name, func(t *testing.T) {
				var failing []string
				for _, r := range gradeRun(context.Background(), c, tc.ev, nil).Results {
					if !r.Pass && !strings.HasPrefix(r.Grader, gradeLLM) {
						failing = append(failing, strings.Fields(r.Grader)[0])
					}
				}
				slices.Sort(failing)
				if !slices.Equal(failing, tc.failing) {
					t.Errorf("failing graders = %v, want %v", failing, tc.failing)
				}
			})
		}
	}
}

// pushed is an origin branch that changes one shared file to content.
func pushed(name, p, content string) OriginBranch {
	return OriginBranch{
		Name:    name,
		Changes: []Change{{Status: "M", Path: p}},
		content: map[string]string{p: content},
	}
}

func readonlyEditRuns() []sharedCaseRun {
	const note = "shared/notes/parcel-tracker.md"
	fix := pushed("brain/edit/parcel-tracker-01jqtv", note, "- HTTP API on port 7071.")
	pr := "pr create --head brain/edit/parcel-tracker-01jqtv --title Fix the parcel-tracker port"
	return []sharedCaseRun{
		{"pull request", Evidence{TwoVault: true, Branches: []OriginBranch{fix}, GHCalls: []string{pr}}, nil},
		{
			"edited in place",
			Evidence{TwoVault: true, Changes: []Change{{Status: "M", Path: note}}},
			[]string{gradeGHCall, gradeOriginBranch, gradeVaultUnchangedExcept},
		},
		{
			"pushed to main",
			Evidence{TwoVault: true, Branches: []OriginBranch{pushed("main", note, "port 7071")}},
			[]string{gradeGHCall, gradeOriginBranch},
		},
	}
}

func privateSearchRuns() []sharedCaseRun {
	search := ToolCall{
		Name: "Bash", Input: `{"command":"brain search dispatch-api"}`, Origin: originMain,
	}
	ev := func(answer string, gh ...string) Evidence {
		return Evidence{TwoVault: true, GHCalls: gh, Transcript: Transcript{
			Result: answer, ToolCalls: []ToolCall{search},
		}}
	}
	named := "Production listens on 8443 (shared:notes/dispatch-api.md); locally you use " +
		"18443 (private:notes/dispatch-api-local-dev.md)."
	return []sharedCaseRun{
		{"names both vaults", ev(named), nil},
		{
			"no vault named",
			ev("8443 per [[dispatch-api]], and 18443 per [[dispatch-api-local-dev]]."),
			[]string{gradeRegex, gradeRegex},
		},
		{"opened a pull request", ev(named, "pr create --title x"), []string{gradeGHCall}},
	}
}

func promoteGateRuns(t *testing.T) []sharedCaseRun {
	const guide = "private/notes/kafka-partitioning-guide.md"
	vaults := func(status string) Evidence {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{guide: "---\ntitle: Kafka partitioning guide\n" +
			"status: " + status + "\nshared_pr: " + fakeGHRepo + "/pull/101\n---\n24 partitions\n"})
		files, err := listFiles(dir)
		if err != nil {
			t.Fatal(err)
		}
		return Evidence{Dir: dir, Files: files, TwoVault: true}
	}
	promoted := pushed(
		"brain/promote/kafka-partitioning-guide-01ky4m",
		"shared/notes/kafka-partitioning-guide.md", "`parcel-events` has 24 partitions.",
	)
	good := vaults("promoting")
	good.Changes = []Change{{Status: "M", Path: guide}}
	good.Branches = []OriginBranch{promoted}
	good.GHCalls = []string{"pr create --head " + promoted.Name}

	both := vaults("promoting")
	both.Changes = []Change{
		{Status: "M", Path: "private/notes/carrier-rate-cards.md"},
		{Status: "M", Path: guide},
	}
	both.Branches = []OriginBranch{
		pushed("brain/promote/carrier-rate-cards-01kf0j",
			"shared/notes/carrier-rate-cards.md", "| CarrierLink | EUR 3.42 |"),
		promoted,
	}
	both.GHCalls = []string{"pr create --head a", "pr create --head b"}
	return []sharedCaseRun{
		{"guide only", good, nil},
		{
			"reclassified and promoted both",
			both,
			[]string{gradeGHCall, gradeOriginBranch, gradeVaultUnchangedExcept},
		},
		{
			"nothing promoted",
			vaults("validated"),
			[]string{gradeFrontmatter, gradeGHCall, gradeOriginBranch},
		},
	}
}

// TestSeedCasesLoad keeps the committed cases loadable: a typo in a grader
// field or a bad regex fails here, not in a paid run.
func TestSeedCasesLoad(t *testing.T) {
	cases, err := loadCases("cases")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 7 {
		t.Errorf("loaded %d seed cases, want at least 7", len(cases))
	}
	for _, c := range cases {
		if !strings.HasSuffix(c.Path, c.ID+".md") {
			t.Errorf("%s: file name does not match id %q", c.Path, c.ID)
		}
	}
	if err := checkFixtures(cases, "fixtures/brain"); err != nil {
		t.Error(err)
	}
}

// TestBrainCasesLoad keeps the brain cases in step with the brain-cli
// profile: every one is named brain-<id>, runs in a brain workspace, and
// the parked multi-vault ones stay pending.
func TestBrainCasesLoad(t *testing.T) {
	cases, err := loadCases("cases")
	if err != nil {
		t.Fatal(err)
	}
	var active, pending int
	for _, c := range cases {
		isBrain := c.Workspace.Kind == workspaceBrain
		if isBrain != strings.HasPrefix(c.ID, "brain-") {
			t.Errorf("%s: a brain case is named brain-<id> and runs in a brain workspace", c.ID)
		}
		switch {
		case !isBrain:
		case c.Pending != "":
			pending++
		default:
			active++
		}
	}
	if active != 15 || pending != 0 {
		t.Errorf("brain cases: %d active, %d pending; want 15 and 0", active, pending)
	}
}

// TestLargeFileGraders pins the ranged-read case to the exact file: a ranged
// read of it passes, an unranged read of it fails, and reads of other files
// count for neither.
func TestLargeFileGraders(t *testing.T) {
	cases, err := loadCases("cases")
	if err != nil {
		t.Fatal(err)
	}
	c, err := selectCases(cases, selection{IDs: "large-file-ranged-read", Profile: profileFull})
	if err != nil {
		t.Fatal(err)
	}
	const file = "/src/thismoon/tools/belt/internal/config/config.go"
	read := func(input string) ToolCall { return ToolCall{Name: "Read", Input: input, Origin: originMain} }
	tests := []struct {
		name        string
		calls       []ToolCall
		used, clean bool
	}{
		{
			"ranged read",
			[]ToolCall{read(`{"file_path":"` + file + `","limit":40,"offset":1060}`)},
			true,
			true,
		},
		{
			"ranged csl_read",
			[]ToolCall{{
				Name: "mcp__csl__csl_read", Origin: originMain,
				Input: `{"end_line":1080,"file":"tools/belt/internal/config/config.go","repo":"thismoon","start_line":1060}`,
			}},
			true, true,
		},
		{"full read", []ToolCall{read(`{"file_path":"` + file + `"}`)}, false, false},
		{
			"ranged read of another file",
			[]ToolCall{read(`{"file_path":"/src/x/config.go","limit":5,"offset":1}`)},
			false,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := Evidence{Transcript: Transcript{ToolCalls: tt.calls}}
			for _, g := range c[0].Graders {
				want, ok := map[string]bool{gradeToolUsed: tt.used, gradeToolNotUsed: tt.clean}[g.Type]
				if !ok {
					continue
				}
				if res, _ := evaluate(context.Background(), g, ev, nil); res.Pass != want {
					t.Errorf("%s: pass = %v, want %v", g.Type, res.Pass, want)
				}
			}
		})
	}
}
