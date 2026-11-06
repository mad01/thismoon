package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/check"
	"github.com/mad01/thismoon/services/brain/internal/hybrid"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/stale"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func nodesByID(g graphView) map[string]graphNode {
	out := map[string]graphNode{}
	for _, n := range g.Nodes {
		out[n.ID] = n
	}
	return out
}

func edgeKinds(g graphView) map[[2]string]string {
	out := map[[2]string]string{}
	for _, e := range g.Edges {
		out[[2]string{e.Source, e.Target}] = e.Kind
	}
	return out
}

func TestGraph(t *testing.T) {
	h := newHarness(t)
	g := getJSON[graphView](h, "/api/graph", http.StatusOK)
	if g.Key == "" || g.Layout.Cached {
		t.Errorf("key %q, cached %v; want a key and no layout yet", g.Key, g.Layout.Cached)
	}
	nodes := nodesByID(g)
	if !nodes[betaNode].Stale || nodes[alphaNode].Stale || nodes[guideNode].Stale {
		t.Errorf("stale: beta %v, alpha %v, guide %v; want beta alone (review_by passed)",
			nodes[betaNode].Stale, nodes[alphaNode].Stale, nodes[guideNode].Stale)
	}
	if a := nodes[alphaNode]; a.Title != "Alpha" || a.Vault != "personal" || a.X != nil {
		t.Errorf("alpha node = %+v, want vault.GraphNode fields and no position", a)
	}
	kinds := edgeKinds(g)
	want := map[[2]string]string{
		{alphaNode, betaNode}:         vault.EdgeLink,
		{alphaNode, guideNode}:        vault.EdgeLink,
		{alphaNode, "dangling:ghost"}: vault.EdgeLink,
		{betaNode, alphaNode}:         vault.EdgeLink,
		{oldNode, alphaNode}:          vault.EdgeSupersedes,
		{mocNode, alphaNode}:          vault.EdgeMOC,
	}
	for pair, kind := range want {
		if kinds[pair] != kind {
			t.Errorf("edge %v kind %q, want %q", pair, kinds[pair], kind)
		}
	}

	topics := getJSON[graphView](h, "/api/graph?topics=1", http.StatusOK)
	if topics.Key == g.Key || nodesByID(topics)["tag:x"].ID == "" {
		t.Errorf(
			"topics=1: key %s (plain %s), want another key and a tag:x node",
			topics.Key,
			g.Key,
		)
	}
	near := getJSON[graphView](h, "/api/graph?center=notes/beta.md&depth=1", http.StatusOK)
	if near.Key != g.Key || len(near.Nodes) != 2 {
		t.Errorf(
			"around beta: key %s, nodes %+v; want the plain key, beta and alpha",
			near.Key,
			near.Nodes,
		)
	}
	// A center names a note of the vault the graph walks.
	teamCopy := getJSON[graphView](h, "/api/graph?vault=team&center=notes/shared.md", http.StatusOK)
	if _, ok := nodesByID(teamCopy)["team:notes/shared.md"]; !ok {
		t.Errorf("team center: nodes %+v, want the team copy", teamCopy.Nodes)
	}
	team := getJSON[graphView](h, "/api/graph?vault=team", http.StatusOK)
	if len(team.Edges) != 0 {
		t.Errorf("team edges = %+v, want none", team.Edges)
	}
}

func TestGraphErrors(t *testing.T) {
	h := newHarness(t)
	for _, q := range []string{"depth=0", "depth=6", "depth=two", "topics=2", "vault=nope"} {
		h.wantError(http.MethodGet, "/api/graph?"+q, nil, http.StatusBadRequest, codeUsage)
	}
	h.wantError(http.MethodGet, "/api/graph?center=notes/shared.md", nil,
		http.StatusBadRequest, codeAmbiguous)
	h.wantError(http.MethodGet, "/api/graph?center=notes/missing.md", nil,
		http.StatusNotFound, codeNotFound)
}

func TestNote(t *testing.T) {
	h := newHarness(t)
	n := getJSON[noteView](h, "/api/note?ref=notes/alpha.md", http.StatusOK)
	if n.Ref != "personal:notes/alpha.md" || n.Vault != "personal" || n.Path != "notes/alpha.md" ||
		n.Frontmatter.Title != "Alpha" || n.Status != "validated" || n.Banner != "" ||
		len(n.SHA) != 64 || !strings.HasPrefix(n.Body, "Alpha is about kubernetes.") {
		t.Errorf("note = %+v", n)
	}
	want := []vault.NoteLink{
		{Text: "[[beta|Bee]]", Target: betaNode, Resolved: true},
		{Text: "[[ghost]]", Target: "dangling:ghost"},
		{Text: "[[Team Guide]]", Target: guideNode, Resolved: true},
	}
	if !slices.Equal(n.Links, want) {
		t.Errorf("links = %+v\nwant %+v", n.Links, want)
	}

	old := getJSON[noteView](h, "/api/note?ref=personal:notes/old.md", http.StatusOK)
	wantBanner := "SUPERSEDED on 2026-01-02 by id:" + idAlpha + ". Cite that note instead."
	if old.Banner != wantBanner || old.Status != "superseded" || len(old.Links) != 0 {
		t.Errorf("old note: banner %q status %q links %v, want %q", old.Banner, old.Status,
			old.Links, wantBanner)
	}
	if byID := getJSON[noteView](h, "/api/note?ref=id:"+idGuide, http.StatusOK); byID.Vault != "team" {
		t.Errorf("by id: %+v", byID)
	}

	h.wantError(http.MethodGet, "/api/note", nil, http.StatusBadRequest, codeUsage)
	h.wantError(
		http.MethodGet,
		"/api/note?ref=notes/missing.md",
		nil,
		http.StatusNotFound,
		codeNotFound,
	)
	h.wantError(http.MethodGet, "/api/note?ref=notes/shared.md", nil,
		http.StatusBadRequest, codeAmbiguous)
	h.wantError(
		http.MethodGet,
		"/api/note?ref=nope:notes/a.md",
		nil,
		http.StatusBadRequest,
		codeUsage,
	)
}

func TestNotes(t *testing.T) {
	h := newHarness(t)
	got := getJSON[vault.Listing](h, "/api/notes?tier=notes", http.StatusOK)
	if got.Tier != "notes" || got.Total != 6 {
		t.Errorf("notes listing = %+v, want six notes", got)
	}
	team := getJSON[vault.Listing](h, "/api/notes?tier=notes&vault=team", http.StatusOK)
	if team.Total != 2 || team.Notes[0].Vault != "team" {
		t.Errorf("team listing = %+v", team)
	}
	raw := getJSON[vault.Listing](
		h,
		"/api/notes?tier=inbox&status=promoting&limit=1",
		http.StatusOK,
	)
	if raw.Total != 1 || len(raw.Notes) != 1 || raw.Notes[0].Path != "inbox/pending.md" {
		t.Errorf("promoting inbox listing = %+v", raw)
	}
	tagged := getJSON[vault.Listing](h, "/api/notes?tier=notes&tag=x", http.StatusOK)
	if tagged.Total != 1 {
		t.Errorf("tag x listing = %+v, want alpha", tagged)
	}
	for _, q := range []string{"", "tier=bogus", "tier=notes&limit=0", "tier=notes&vault=nope"} {
		h.wantError(http.MethodGet, "/api/notes?"+q, nil, http.StatusBadRequest, codeUsage)
	}
}

func TestBacklinks(t *testing.T) {
	h := newHarness(t)
	got := getJSON[vault.Backlinks](h, "/api/backlinks?ref=notes/alpha.md", http.StatusOK)
	var from []string
	for _, in := range got.Inbound {
		from = append(from, in.Vault+":"+in.Path)
	}
	slices.Sort(from)
	want := []string{mocNode, betaNode}
	if !slices.Equal(from, want) || len(got.Outbound) != 3 {
		t.Errorf(
			"inbound %v, outbound %+v; want %v and alpha's three links",
			from,
			got.Outbound,
			want,
		)
	}
	h.wantError(http.MethodGet, "/api/backlinks", nil, http.StatusBadRequest, codeUsage)
	h.wantError(http.MethodGet, "/api/backlinks?ref=notes/missing.md", nil,
		http.StatusNotFound, codeNotFound)
}

func TestSearch(t *testing.T) {
	h := newHarness(t)
	before := vaultFiles(t, h.roots)
	got := getJSON[search.Result](h, "/api/search?q=kubernetes", http.StatusOK)
	var hits []string
	for _, f := range got.Files {
		hits = append(hits, f.Vault+":"+f.Path)
	}
	slices.Sort(hits)
	want := []string{"personal:notes/alpha.md", "team:notes/team-guide.md"}
	if got.OutputMode != search.ModeFiles || !slices.Equal(hits, want) {
		t.Errorf("search = %+v, want files %v", got, want)
	}
	team := getJSON[search.Result](h, "/api/search?q=kubernetes&vault=team&tier=notes&limit=5",
		http.StatusOK)
	if team.Total != 1 {
		t.Errorf("team search = %+v", team)
	}
	if _, err := os.Stat(filepath.Join(h.state, "index", "personal", "state.json")); err != nil {
		t.Errorf("search did not refresh the literal index: %v", err)
	}
	if after := vaultFiles(t, h.roots); !slices.Equal(before, after) {
		t.Errorf("search changed the vaults:\nbefore %v\nafter  %v", before, after)
	}
	if locks, _ := filepath.Glob(filepath.Join(h.state, "locks", "*.write.lock")); len(locks) > 0 {
		t.Errorf("search took a write lock: %v", locks)
	}

	h.wantError(http.MethodGet, "/api/search?q=foo(", nil, http.StatusBadRequest, codeBadQuery)
	h.wantError(http.MethodGet, "/api/search?q=kubernetes&mode=hybrid", nil,
		http.StatusServiceUnavailable, codeNoSemantic)
	for _, q := range []string{
		"", "q=+", "q=x&mode=fuzzy", "q=x&include_revoked=yes", "q=x&limit=-1", "q=x&tier=bogus",
		"q=x&vault=nope",
	} {
		h.wantError(http.MethodGet, "/api/search?"+q, nil, http.StatusBadRequest, codeUsage)
	}
}

func TestHybridSearchShape(t *testing.T) {
	// A semantic layer that is on but whose Ollama fails still returns the
	// hybrid shape, with literal hits and a warning.
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ollama.Close()
	h := newHarnessWith(t, "semantic:\n  enabled: true\n  ollama_url: "+ollama.URL+"\n")
	got := getJSON[hybrid.Result](h, "/api/search?q=kubernetes&mode=hybrid&limit=5", http.StatusOK)
	if got.Total == 0 || len(got.Warnings) == 0 {
		t.Errorf("hybrid = %+v, want literal hits and a warning", got)
	}
}

// vaultFiles lists every file under the vault roots with its content.
func vaultFiles(t *testing.T, roots map[string]string) []string {
	t.Helper()
	var out []string
	for name, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			out = append(out, name+":"+rel+"="+string(data))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	slices.Sort(out)
	return out
}

func TestStale(t *testing.T) {
	h := newHarness(t)
	rec := h.do(http.MethodGet, "/api/stale", nil)
	if !strings.Contains(rec.Body.String(), `"drift":false`) {
		t.Errorf("stale body %s lacks \"drift\":false", rec.Body)
	}
	q := decode[stale.Queue](t, rec, http.StatusOK)
	if q.Total != 1 || q.Groups[0].Notes[0].Path != "notes/beta.md" {
		t.Errorf("stale = %+v, want beta alone", q)
	}
}

func TestCheck(t *testing.T) {
	h := newHarness(t)
	r := getJSON[check.Report](h, "/api/check", http.StatusOK)
	if r.Checked != 8 || r.Findings == nil {
		t.Errorf("check = %+v, want every note of both vaults checked", r)
	}
}

func TestPromotions(t *testing.T) {
	h := newHarness(t)
	got := getJSON[promotionsView](h, "/api/promotions", http.StatusOK)
	want := promotion{
		Vault: "personal", Path: "inbox/pending.md", ID: idPending, Title: "Pending",
		SharedPR: "https://example.com/pr/1", StatusAt: "2026-02-03",
	}
	if len(got.Promotions) != 1 || got.Promotions[0] != want {
		t.Errorf("promotions = %+v, want %+v", got.Promotions, want)
	}
}

func TestEveryAPIRouteAnswersJSON(t *testing.T) {
	h := newHarness(t)
	for _, target := range []string{
		"/api/vaults", "/api/state", "/api/graph", "/api/notes?tier=notes",
		"/api/note?ref=notes/beta.md", "/api/backlinks?ref=notes/beta.md",
		"/api/search?q=beta", "/api/stale", "/api/check", "/api/promotions", "/api/doctor",
	} {
		rec := h.do(http.MethodGet, target, nil)
		decode[map[string]any](t, rec, http.StatusOK)
	}
}

func TestDoctor(t *testing.T) {
	h := newHarness(t)
	got := getJSON[DoctorView](h, "/api/doctor", http.StatusOK)
	if got.StateDir != h.state || !got.LayoutDirWritable || got.ConfigError != "" ||
		len(got.Vaults) != 2 {
		t.Fatalf("doctor = %+v, want state %s", got, h.state)
	}
	for _, v := range got.Vaults {
		if !v.Readable || v.Error != "" || v.Path != h.roots[v.Name] || v.Resolved == "" {
			t.Errorf("vault %+v, want readable at %s", v, h.roots[v.Name])
		}
	}
	files, err := os.ReadDir(filepath.Join(h.state, "layout"))
	if err != nil || len(files) != 0 {
		t.Errorf("doctor left %v in the layout directory (%v)", files, err)
	}
}

func TestDoctorNamesAnUnreadableVault(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode 000 directory")
	}
	h := newHarness(t)
	team := h.roots["team"]
	if err := os.Chmod(team, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(team, 0o755) })

	got := getJSON[DoctorView](h, "/api/doctor", http.StatusOK)
	for _, v := range got.Vaults {
		wantReadable := v.Name != "team"
		if v.Readable != wantReadable || (v.Error == "") != wantReadable {
			t.Errorf("vault %+v, want readable %v with an error when not", v, wantReadable)
		}
	}
	g := getJSON[graphView](h, "/api/graph", http.StatusOK)
	if _, ok := nodesByID(g)[alphaNode]; !ok {
		t.Errorf("graph without the unreadable vault lacks alpha: %+v", g.Nodes)
	}
	vaults := getJSON[vaultsView](h, "/api/vaults", http.StatusOK)
	if vaults.Vaults[1].Name != "team" || vaults.Vaults[1].Notes != 0 {
		t.Errorf("vaults = %+v, want team listed with no notes", vaults.Vaults)
	}
	h.wantError(http.MethodGet, "/api/search?q=x&vault=team", nil, http.StatusBadRequest, codeUsage)
}

func TestNoVault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	missing := filepath.Join(home, "config.yaml")
	h := openHarness(t, Options{ConfigPath: missing, ConfigErr: os.ErrNotExist}, nil,
		filepath.Join(home, "state", "brain"))

	if v := getJSON[vaultsView](h, "/api/vaults", http.StatusOK); v.Vaults == nil ||
		len(v.Vaults) != 0 {
		t.Errorf("vaults = %+v, want an empty list", v)
	}
	d := getJSON[DoctorView](h, "/api/doctor", http.StatusOK)
	if d.Config != missing || d.ConfigError == "" || d.Vaults == nil || len(d.Vaults) != 0 {
		t.Errorf("doctor = %+v, want the config error and no vaults", d)
	}
	g := getJSON[graphView](h, "/api/graph", http.StatusOK)
	if g.Key == "" || len(g.Nodes) != 0 || g.Nodes == nil {
		t.Errorf("graph = %+v, want a key and no nodes", g)
	}
	getJSON[search.Result](h, "/api/search?q=x", http.StatusOK)
	getJSON[stale.Queue](h, "/api/stale", http.StatusOK)
	getJSON[check.Report](h, "/api/check", http.StatusOK)
	h.wantError(http.MethodGet, "/api/note?ref=notes/a.md", nil, http.StatusNotFound, codeNotFound)
	if rec := h.do(http.MethodGet, "/healthz", nil); rec.Code != http.StatusOK {
		t.Errorf("healthz = %d", rec.Code)
	}
}

func TestMissingVaultDirectory(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	h := newHarnessWith(t, "  - {name: gone, path: "+gone+"}\n")
	d := getJSON[DoctorView](h, "/api/doctor", http.StatusOK)
	if len(d.Vaults) != 3 {
		t.Fatalf("doctor vaults = %+v, want three", d.Vaults)
	}
	if last := d.Vaults[2]; last.Name != "gone" || last.Readable || last.Error == "" ||
		last.Resolved != gone {
		t.Errorf("missing vault = %+v", last)
	}
	if v := getJSON[vaultsView](h, "/api/vaults", http.StatusOK); len(v.Vaults) != 2 {
		t.Errorf("vaults = %+v, want the two that open", v.Vaults)
	}
}

func TestQueryEscaping(t *testing.T) {
	h := newHarness(t)
	target := "/api/note?ref=" + url.QueryEscape("personal:notes/alpha.md")
	if n := getJSON[noteView](h, target, http.StatusOK); n.Path != "notes/alpha.md" {
		t.Errorf("note = %+v", n)
	}
}
