package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/doctor"
	"github.com/mad01/thismoon/services/brain/internal/hybrid"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

var fixtureNotes = map[string]string{
	"notes/alpha.md": `---
title: Alpha
aliases: [First]
tags: [ai]
evidence: A
owner: alex
updated: 2026-04-02
---
Alpha body about kubernetes.
Links [[beta]].
`,
	"notes/beta.md": `---
title: Beta
tags: [ai]
updated: 2026-04-01
---
Beta extends [[alpha]].
`,
	"inbox/raw.md": `---
owner: sam
title: Raw Capture
classification: internal
sources:
  - url: https://example.com/a
    note: primary
status: raw
---
A kubernetes capture, see [[alpha]].
`,
	"inbox/dup.md": `---
title: Alpha
status: raw
---
Collides with notes/alpha.md on promote.
`,
}

// setupCLI isolates HOME and the XDG and brain variables, writes a brain
// config at the default path for a fresh fixture vault and returns the vault
// root. extraConfig is appended to the config file.
func setupCLI(t *testing.T, extraConfig string) string {
	t.Helper()
	home := testhome.Isolate(t)
	root := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "brain")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "vault: " + root + "\n" + extraConfig
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	seedVault(t, root)
	return root
}

// seedVault creates the tier directories under root and writes fixtureNotes.
func seedVault(t *testing.T, root string) {
	t.Helper()
	for _, tier := range vault.AllTiers {
		if err := os.MkdirAll(filepath.Join(root, tier.Dir()), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for rel, content := range fixtureNotes {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type result struct {
	stdout, stderr string
	code           int
}

func runBrain(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, strings.NewReader(stdin), &stdout, &stderr)
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// decodeStrict decodes data into T and fails on any field T does not have,
// proving the CLI prints exactly the shared shape.
func decodeStrict[T any](t *testing.T, data string) T {
	t.Helper()
	var v T
	dec := json.NewDecoder(strings.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %T: %v\n%s", v, err, data)
	}
	return v
}

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string // prefix of the single stderr line
	}{
		{"read ok", []string{"read", "notes/alpha.md"}, exitOK, ""},
		{
			"read missing",
			[]string{"read", "does-not-exist.md"},
			exitNotFound,
			"brain read: vault: note not found",
		},
		{
			"promote missing",
			[]string{"promote", "nope.md"},
			exitNotFound,
			"brain promote: vault: note not found",
		},
		{
			"edit missing",
			[]string{"edit", "notes/nope.md", "--body", "x"},
			exitNotFound,
			"brain edit: vault: note not found",
		},
		{
			"backlinks title missing",
			[]string{"backlinks", "--title", "Nope"},
			exitNotFound,
			"brain backlinks: vault: note not found",
		},
		{
			"unknown tier",
			[]string{"search", "--tier", "bogus", "foo"},
			exitUsage,
			`brain search: vault: unknown tier "bogus"`,
		},
		{"unknown command", []string{"bogus"}, exitUsage, `brain: unknown command "bogus"`},
		{
			"unknown flag",
			[]string{"read", "--bogus", "x"},
			exitUsage,
			"brain read: unknown flag: --bogus",
		},
		{"missing argument", []string{"read"}, exitUsage, "brain read: want 1 argument(s), got 0"},
		{
			"path escapes vault",
			[]string{"read", "../x.md"},
			exitUsage,
			"brain read: vault: invalid path",
		},
		{
			"edit nothing to do",
			[]string{"edit", "notes/alpha.md"},
			exitUsage,
			"brain edit: nothing to do",
		},
		{
			"set wrong shape",
			[]string{"edit", "notes/alpha.md", "--set", "title=[a]"},
			exitUsage,
			"brain edit: --set title: frontmatter title takes a string, got a list",
		},
		{
			"set without equals",
			[]string{"write", "--title", "X", "--set", "owner"},
			exitUsage,
			`brain write: --set "owner"`,
		},
		{
			"write without title",
			[]string{"write", "--body", "x"},
			exitUsage,
			"brain write: --title is required",
		},
		{
			"bad output format",
			[]string{"read", "notes/alpha.md", "-o", "yaml"},
			exitUsage,
			"brain read: -o must be",
		},
		{"list without tier", []string{"list"}, exitUsage, "brain list: name a tier"},
		{
			"write existing",
			[]string{"write", "--tier", "notes", "--title", "Alpha", "--body", "x"},
			exitExists,
			"brain write: vault: note already exists",
		},
		{
			"promote onto existing",
			[]string{"promote", "inbox/dup.md"},
			exitExists,
			"brain promote: vault: note already exists",
		},
		{
			"invalid search query",
			[]string{"search", "foo("},
			exitInvalidQuery,
			"brain search: search: invalid query",
		},
		{
			"invalid count query",
			[]string{"count", "foo("},
			exitInvalidQuery,
			"brain count: search: invalid query",
		},
		{
			"invalid query",
			[]string{"query", "foo("},
			exitInvalidQuery,
			"brain query: search: invalid query",
		},
		{
			"semantic disabled",
			[]string{"semantic-search", "x"},
			exitError,
			"brain semantic-search: semantic.enabled is false",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupCLI(t, "")
			got := runBrain(t, "", tc.args...)
			if got.code != tc.wantCode {
				t.Fatalf("exit = %d, want %d; stderr %q", got.code, tc.wantCode, got.stderr)
			}
			if tc.wantCode == exitOK {
				if got.stderr != "" {
					t.Errorf("stderr = %q, want empty", got.stderr)
				}
				return
			}
			if strings.Count(got.stderr, "\n") != 1 ||
				!strings.HasPrefix(got.stderr, tc.wantStderr) {
				t.Errorf("stderr = %q, want one line starting %q", got.stderr, tc.wantStderr)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty in text mode", got.stdout)
			}
		})
	}
}

func TestMissingConfigIsGenericError(t *testing.T) {
	testhome.Isolate(t)
	got := runBrain(t, "", "read", "notes/alpha.md")
	if got.code != exitError || !strings.Contains(got.stderr, "brain doctor") {
		t.Errorf("exit %d, stderr %q; want 1 pointing at brain doctor", got.code, got.stderr)
	}
}

func TestNotFoundHintNamesTier(t *testing.T) {
	setupCLI(t, "")
	got := runBrain(t, "", "read", "notes/nope.md")
	want := "(find it with: brain list --tier notes, or brain search 'f:nope')"
	if !strings.Contains(got.stderr, want) {
		t.Errorf("stderr = %q, want hint %q", got.stderr, want)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	tests := []struct {
		args     []string
		wantCode string
	}{
		{[]string{"read", "nope.md"}, "not_found"},
		{[]string{"search", "--tier", "bogus", "x"}, "usage"},
		{[]string{"read", "--bogus"}, "usage"},
		{[]string{"write", "--tier", "notes", "--title", "Alpha", "--body", "x"}, "already_exists"},
		{[]string{"query", "foo("}, "invalid_query"},
		{[]string{"semantic-search", "x"}, "error"},
	}
	for _, tc := range tests {
		t.Run(tc.wantCode, func(t *testing.T) {
			setupCLI(t, "")
			got := runBrain(t, "", append(tc.args, "-o", "json")...)
			env := decodeStrict[errorEnvelope](t, got.stdout)
			if env.Error.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", env.Error.Code, tc.wantCode)
			}
			if !strings.HasSuffix(strings.TrimSpace(got.stderr), env.Error.Message) {
				t.Errorf("stderr %q does not end with message %q", got.stderr, env.Error.Message)
			}
		})
	}
}

func TestOutputFromArgs(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"read", "x", "-o", "json"}, "json"},
		{[]string{"read", "--output=json"}, "json"},
		{[]string{"read", "-ojson"}, "json"},
		{[]string{"read", "-o=json", "-o", "text"}, "text"},
		{[]string{"read", "--", "-o", "json"}, ""},
		{[]string{"read", "-o"}, ""},
	}
	for _, tc := range tests {
		if got := outputFromArgs(tc.args); got != tc.want {
			t.Errorf("outputFromArgs(%q) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestJSONOutput(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin string
		check func(t *testing.T, out string)
	}{
		{
			"read",
			[]string{"read", "notes/alpha.md", "--start", "2"},
			"",
			func(t *testing.T, out string) {
				n := decodeStrict[vault.Note](t, out)
				if n.Frontmatter.Evidence != "A" || n.Frontmatter.Extra["owner"] != "alex" ||
					len(n.BodyLines) != 1 || n.BodyLines[0].Line != 2 {
					t.Errorf("note = %+v", n)
				}
			},
		},
		{
			"peek",
			[]string{"peek", "notes/alpha.md", "--lines", "1"},
			"",
			func(t *testing.T, out string) {
				p := decodeStrict[vault.Peek](t, out)
				if p.BacklinkCount != 2 || len(p.Preview) != 1 {
					t.Errorf("peek = %+v", p)
				}
			},
		},
		{"search files", []string{"search", "kubernetes"}, "", func(t *testing.T, out string) {
			r := decodeStrict[search.Result](t, out)
			if r.OutputMode != search.ModeFiles || r.Total != 2 || len(r.Files) != 2 {
				t.Errorf("result = %+v", r)
			}
		}},
		{
			"search content",
			[]string{"search", "kubernetes", "-m", "content", "--tier", "notes"},
			"",
			func(t *testing.T, out string) {
				r := decodeStrict[search.Result](t, out)
				if r.Total != 1 || r.Lines[0].Path != "notes/alpha.md" ||
					r.Lines[0].Evidence != "A" {
					t.Errorf("result = %+v", r)
				}
			},
		},
		{
			"count",
			[]string{"count", "kubernetes", "--group-by", "tier"},
			"",
			func(t *testing.T, out string) {
				c := decodeStrict[search.Counts](t, out)
				if c.Total != 2 || len(c.Groups) != 2 {
					t.Errorf("counts = %+v", c)
				}
			},
		},
		{"query", []string{"query", "foo bar"}, "", func(t *testing.T, out string) {
			if q := decodeStrict[search.QueryInfo](t, out); !q.Valid || q.Parsed == "" {
				t.Errorf("query = %+v", q)
			}
		}},
		{"list", []string{"list", "notes"}, "", func(t *testing.T, out string) {
			l := decodeStrict[vault.Listing](t, out)
			if l.Tier != "notes" || l.Total != 2 || l.Notes[0].Path != "notes/alpha.md" {
				t.Errorf("listing = %+v", l)
			}
		}},
		{
			"list --tier",
			[]string{"list", "--tier", "inbox", "--limit", "1"},
			"",
			func(t *testing.T, out string) {
				if l := decodeStrict[vault.Listing](t, out); l.Total != 2 || len(l.Notes) != 1 {
					t.Errorf("listing = %+v", l)
				}
			},
		},
		{"backlinks", []string{"backlinks", "notes/alpha.md"}, "", func(t *testing.T, out string) {
			b := decodeStrict[vault.Backlinks](t, out)
			if b.NotePath != "notes/alpha.md" || len(b.Inbound) != 2 || len(b.Outbound) != 1 ||
				b.Outbound[0].Path != "notes/beta.md" {
				t.Errorf("backlinks = %+v", b)
			}
		}},
		{
			"backlinks --title",
			[]string{"backlinks", "--title", "first"},
			"",
			func(t *testing.T, out string) {
				if b := decodeStrict[vault.Backlinks](t, out); b.NotePath != "notes/alpha.md" {
					t.Errorf("backlinks = %+v", b)
				}
			},
		},
		{
			"write",
			[]string{"write", "--title", "New Note", "--evidence", "C", "--set", "owner=kim"},
			"body\n",
			func(t *testing.T, out string) {
				if p := decodeStrict[vault.PathResult](t, out); p.Path != "inbox/new-note.md" {
					t.Errorf("path = %+v", p)
				}
			},
		},
		{
			"edit",
			[]string{"edit", "notes/beta.md", "--append", "more"},
			"",
			func(t *testing.T, out string) {
				if p := decodeStrict[vault.PathResult](t, out); p.Path != "notes/beta.md" {
					t.Errorf("path = %+v", p)
				}
			},
		},
		{"promote", []string{"promote", "inbox/raw.md"}, "", func(t *testing.T, out string) {
			r := decodeStrict[vault.PromoteReport](t, out)
			if r.OldPath != "inbox/raw.md" || r.NewPath != "notes/raw-capture.md" {
				t.Errorf("report = %+v", r)
			}
		}},
		{"version", []string{"version"}, "", func(t *testing.T, out string) {
			if v := decodeStrict[map[string]string](t, out); v["version"] == "" {
				t.Errorf("version = %v", v)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupCLI(t, "")
			got := runBrain(t, tc.stdin, append(tc.args, "-o", "json")...)
			if got.code != exitOK {
				t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
			}
			tc.check(t, got.stdout)
		})
	}
}

// fakeOllama serves /api/tags and a deterministic /api/embeddings so the
// semantic commands run without a real Ollama.
func fakeOllama(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"models":[]}`)
	})
	mux.HandleFunc("/api/embeddings", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string][]float32{"embedding": hashVector(req.Prompt)}); err != nil {
			t.Errorf("fake ollama encode: %v", err)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// hashVector is a 16-dim bag-of-words vector: texts sharing words are close.
func hashVector(text string) []float32 {
	v := make([]float32, 16)
	for _, w := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New32a()
		h.Write([]byte(w))
		v[h.Sum32()%16]++
	}
	var sum float64
	for _, x := range v {
		sum += float64(x * x)
	}
	if sum == 0 {
		v[0] = 1
		return v
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / math.Sqrt(sum))
	}
	return v
}

func TestSemanticJSONOutput(t *testing.T) {
	srv := fakeOllama(t)
	setupCLI(t, "semantic:\n  enabled: true\n  ollama_url: "+srv.URL+"\n")
	if got := runBrain(t, "", "index"); got.code != exitOK {
		t.Fatalf("index: exit %d, stderr %q", got.code, got.stderr)
	}

	got := runBrain(t, "", "semantic-search", "kubernetes capture", "-o", "json")
	if got.code != exitOK {
		t.Fatalf("semantic-search: exit %d, stderr %q", got.code, got.stderr)
	}
	sem := decodeStrict[semantic.SearchResult](t, got.stdout)
	if sem.OutputMode != semantic.ModeSnippets || sem.Total == 0 || sem.Hits[0].Path == "" {
		t.Errorf("semantic result = %+v", sem)
	}

	got = runBrain(t, "", "hybrid-search", "kubernetes", "-o", "json")
	if got.code != exitOK {
		t.Fatalf("hybrid-search: exit %d, stderr %q", got.code, got.stderr)
	}
	hyb := decodeStrict[hybrid.Result](t, got.stdout)
	if hyb.Total == 0 || len(hyb.Hits[0].Sources) == 0 {
		t.Errorf("hybrid result = %+v", hyb)
	}
}

// TestSemanticRefreshesAStaleIndex covers the CLI query path: a stale
// semantic index is synced before the query, and with Ollama gone the
// hybrid search still answers from the literal side and says why on stderr.
func TestSemanticRefreshesAStaleIndex(t *testing.T) {
	srv := fakeOllama(t)
	root := setupCLI(t, "semantic:\n  enabled: true\n  ollama_url: "+srv.URL+"\n")
	if got := runBrain(t, "", "index"); got.code != exitOK {
		t.Fatalf("index: exit %d, stderr %q", got.code, got.stderr)
	}
	note := "---\ntitle: Late\n---\nlatecomer walrus\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "late.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runBrain(t, "", "semantic-search", "latecomer walrus", "-o", "json")
	sem := decodeStrict[semantic.SearchResult](t, got.stdout)
	if got.code != exitOK || sem.Total == 0 || sem.Hits[0].Path != "notes/late.md" {
		t.Errorf(
			"semantic-search after an edit: exit %d, hits %+v, stderr %q",
			got.code,
			sem.Hits,
			got.stderr,
		)
	}

	if err := os.WriteFile(filepath.Join(root, "notes", "later.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	got = runBrain(t, "", "hybrid-search", "walrus")
	for _, want := range []string{
		"warning: default: semantic index is stale and was not refreshed",
		"warning: semantic search failed, hits are literal only",
	} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr %q lacks %q", got.stderr, want)
		}
	}
	if got.code != exitOK || !strings.Contains(got.stdout, "notes/later.md") {
		t.Errorf("hybrid-search without Ollama: exit %d, stdout %q", got.code, got.stdout)
	}
}

// TestSearchLeavesZoektSilent pins that the zoekt index build and shard load,
// which log through the standard logger, print nothing: stderr carries only
// brain's own messages.
func TestSearchLeavesZoektSilent(t *testing.T) {
	setupCLI(t, "")
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	got := runBrain(t, "", "search", "kubernetes")
	if got.code != exitOK || got.stderr != "" {
		t.Fatalf("exit %d, stderr %q; want 0 and empty", got.code, got.stderr)
	}
	if logged.Len() != 0 {
		t.Errorf("the standard logger got %q, want nothing", logged.String())
	}
}

func TestTextOutputBacklinksAndPeek(t *testing.T) {
	setupCLI(t, "")
	got := runBrain(t, "", "backlinks", "notes/alpha.md")
	for _, want := range []string{
		"inbound (2):",
		"  inbox/raw.md:10: A kubernetes capture, see [[alpha]].",
		"  notes/beta.md:6: Beta extends [[alpha]].",
		"outbound (1):",
		"  10: [[beta]] -> notes/beta.md",
	} {
		if !strings.Contains(got.stdout, want+"\n") {
			t.Errorf("backlinks text missing %q:\n%s", want, got.stdout)
		}
	}
	got = runBrain(t, "", "peek", "notes/alpha.md", "--lines", "1")
	want := "backlinks: 2\n---\n"
	if !strings.HasPrefix(got.stdout, want) ||
		!strings.HasSuffix(got.stdout, "---\nAlpha body about kubernetes.\n") {
		t.Errorf("peek text = %q", got.stdout)
	}
	if !strings.Contains(got.stdout, "owner: alex\n") {
		t.Errorf("peek text drops unknown keys: %q", got.stdout)
	}
}

func TestParseSets(t *testing.T) {
	tests := []struct {
		name    string
		raw     []string
		wantKey string
		wantTag string // ShortTag of the parsed value
		wantErr bool
	}{
		{name: "scalar", raw: []string{"owner=alex"}, wantKey: "owner", wantTag: "!!str"},
		{name: "list", raw: []string{"tags=[a,b]"}, wantKey: "tags", wantTag: "!!seq"},
		{
			name:    "nested",
			raw:     []string{"sources=[{url: https://x}]"},
			wantKey: "sources",
			wantTag: "!!seq",
		},
		{name: "value keeps later equals", raw: []string{"q=a=b"}, wantKey: "q", wantTag: "!!str"},
		{name: "key is trimmed", raw: []string{" owner =alex"}, wantKey: "owner", wantTag: "!!str"},
		{name: "missing equals", raw: []string{"owner"}, wantErr: true},
		{name: "empty key", raw: []string{"=x"}, wantErr: true},
		{name: "empty value", raw: []string{"owner="}, wantErr: true},
		{name: "bad yaml", raw: []string{"tags=[a"}, wantErr: true},
		{name: "typed key wrong shape", raw: []string{"tags={a: b}"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields, err := parseSets(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				if code, _ := classify(err); code != exitUsage {
					t.Errorf("exit code = %d, want usage", code)
				}
				return
			}
			if len(fields) != 1 || fields[0].Key != tc.wantKey ||
				fields[0].Value.ShortTag() != tc.wantTag {
				t.Errorf("fields = %+v", fields)
			}
		})
	}
	if _, err := parseUnsets([]string{"owner", " "}); err == nil {
		t.Error("parseUnsets accepted an empty key")
	}
}

func TestEditAndPromoteKeepUnknownKeys(t *testing.T) {
	root := setupCLI(t, "")
	got := runBrain(
		t,
		"",
		"edit",
		"inbox/raw.md",
		"--set",
		"review={by: kim}",
		"--set",
		"owner=lee",
		"--unset",
		"classification",
		"--evidence",
		"B",
	)
	if got.code != exitOK {
		t.Fatalf("edit: exit %d, stderr %q", got.code, got.stderr)
	}
	if got = runBrain(t, "", "promote", "inbox/raw.md"); got.code != exitOK {
		t.Fatalf("promote: exit %d, stderr %q", got.code, got.stderr)
	}
	data, err := os.ReadFile(filepath.Join(root, "notes", "raw-capture.md"))
	if err != nil {
		t.Fatal(err)
	}
	fm, _, err := markdown.ParseFrontmatter(data)
	if err != nil {
		t.Fatal(err)
	}
	extra, err := fm.ExtraValues()
	if err != nil {
		t.Fatal(err)
	}
	if extra["owner"] != "lee" || extra["classification"] != nil || extra["review"] == nil ||
		extra["sources"] == nil {
		t.Errorf("extra = %#v", extra)
	}
	if fm.Evidence != "B" || fm.Status != "validated" {
		t.Errorf("typed = %+v", fm)
	}
	if !strings.HasPrefix(string(data), "---\nowner: lee\ntitle: Raw Capture\nsources:\n") {
		t.Errorf("source key order lost:\n%s", data)
	}
}

func TestDoctor(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	tests := []struct {
		name       string
		setup      func(t *testing.T)
		wantCode   int
		wantStatus map[string]string
	}{
		{
			name: "healthy",
			setup: func(t *testing.T) {
				setupCLI(t, "")
				if got := runBrain(t, "", "index"); got.code != exitOK {
					t.Fatalf("index: %q", got.stderr)
				}
			},
			wantCode: exitOK,
			wantStatus: map[string]string{
				"config": doctor.StatusOK, "vault": doctor.StatusOK, "index-dir": doctor.StatusOK, "index": doctor.StatusOK, "ids": doctor.StatusOK, "ollama": doctor.StatusSkip,
			},
		},
		{
			name:     "index never built",
			setup:    func(t *testing.T) { setupCLI(t, "") },
			wantCode: exitError,
			wantStatus: map[string]string{
				"config": doctor.StatusOK, "vault": doctor.StatusOK, "index-dir": doctor.StatusOK, "index": doctor.StatusFail, "ids": doctor.StatusOK, "ollama": doctor.StatusSkip,
			},
		},
		{
			name: "vault missing",
			setup: func(t *testing.T) {
				root := setupCLI(t, "")
				if err := os.RemoveAll(root); err != nil {
					t.Fatal(err)
				}
			},
			wantCode: exitError,
			wantStatus: map[string]string{
				"config": doctor.StatusOK, "vault": doctor.StatusFail, "index-dir": doctor.StatusOK, "index": doctor.StatusSkip, "ids": doctor.StatusSkip, "ollama": doctor.StatusSkip,
			},
		},
		{
			name:     "no config",
			setup:    func(t *testing.T) { testhome.Isolate(t) },
			wantCode: exitError,
			wantStatus: map[string]string{
				"config": doctor.StatusFail, "vault": doctor.StatusSkip, "index-dir": doctor.StatusSkip, "index": doctor.StatusSkip, "ids": doctor.StatusSkip, "ollama": doctor.StatusSkip,
			},
		},
		{
			name: "duplicate ids",
			setup: func(t *testing.T) {
				root := setupCLI(t, "")
				for _, name := range []string{"one", "two"} {
					note := "---\ntitle: " + name + "\nid: 01JDUPDUPDUPDUPDUPDUPDUPDU\n---\nbody\n"
					path := filepath.Join(root, "notes", name+".md")
					if err := os.WriteFile(path, []byte(note), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				if got := runBrain(t, "", "index"); got.code != exitOK {
					t.Fatalf("index: %q", got.stderr)
				}
			},
			wantCode: exitError,
			wantStatus: map[string]string{
				"config": doctor.StatusOK, "vault": doctor.StatusOK, "index-dir": doctor.StatusOK, "index": doctor.StatusOK, "ids": doctor.StatusFail, "ollama": doctor.StatusSkip,
			},
		},
		{
			name:     "ollama down",
			setup:    func(t *testing.T) { setupCLI(t, "semantic:\n  enabled: true\n  ollama_url: "+dead.URL+"\n") },
			wantCode: exitError,
			wantStatus: map[string]string{
				"config": doctor.StatusOK, "vault": doctor.StatusOK, "index-dir": doctor.StatusOK, "index": doctor.StatusFail, "ids": doctor.StatusOK, "ollama": doctor.StatusFail,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			got := runBrain(t, "", "doctor", "-o", "json")
			if got.code != tc.wantCode {
				t.Fatalf("exit %d, want %d; stderr %q", got.code, tc.wantCode, got.stderr)
			}
			report := decodeStrict[doctor.Report](t, got.stdout)
			if report.OK != (tc.wantCode == exitOK) {
				t.Errorf("ok = %v", report.OK)
			}
			status := map[string]string{}
			for _, c := range report.Checks {
				status[c.Name] = c.Status
				if c.Status == doctor.StatusFail && c.Fix == "" && c.Name != "index" {
					t.Errorf("check %s failed without a fix", c.Name)
				}
			}
			if !mapsEqual(status, tc.wantStatus) {
				t.Errorf("statuses = %v, want %v", status, tc.wantStatus)
			}
			text := runBrain(t, "", "doctor")
			if lines := strings.Count(text.stdout, "\n"); lines != len(tc.wantStatus) {
				t.Errorf(
					"text doctor printed %d lines, want one per check:\n%s",
					lines,
					text.stdout,
				)
			}
		})
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestDocs(t *testing.T) {
	got := runBrain(t, "", "docs")
	if got.code != exitOK {
		t.Fatalf("exit %d", got.code)
	}
	for _, want := range []string{"## Exit codes", "## JSON shapes", "--set", "promote"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("docs missing %q", want)
		}
	}
	if n := strings.Count(got.stdout, "\n"); n >= 200 {
		t.Errorf("docs is %d lines, keep it under 200", n)
	}
	if !strings.Contains(got.stdout, "brain docs lifecycle") {
		t.Error("docs does not point at brain docs lifecycle")
	}
}

func TestDocsLifecycle(t *testing.T) {
	got := runBrain(t, "", "docs", "lifecycle")
	if got.code != exitOK {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	for _, want := range []string{"## Statuses", "## brain check", "expires_at"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("docs lifecycle missing %q", want)
		}
	}
	if n := strings.Count(got.stdout, "\n"); n >= 200 {
		t.Errorf("docs lifecycle is %d lines, keep it under 200", n)
	}
	if got := runBrain(t, "", "docs", "bogus"); got.code != exitUsage {
		t.Errorf("docs bogus: exit %d, want %d", got.code, exitUsage)
	}
}
