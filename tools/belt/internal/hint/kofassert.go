package hint

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// maxAssertions caps how many assertions one hint fires with. Past a handful
// the block stops being read, and an unread hint is a silent failure: nothing
// errors, the advice is simply ignored (docs/adr/0008).
const maxAssertions = 3

// kofTimeout bounds the call to kof serve. The search already returned, so
// a slow or dead kof costs nothing but this budget and then stays quiet.
const kofTimeout = 400 * time.Millisecond

// KofAssertions surfaces stored assertions whose subject matches the code a
// csl search just looked at. It exists because assertions only pay off when a
// later session reads them, and nothing was prompting that read.
type KofAssertions struct {
	cfg  config.Config
	base string // kof serve base URL; overridable for tests
}

func NewKofAssertions(cfg config.Config) *KofAssertions {
	return &KofAssertions{cfg: cfg, base: kofBaseURL()}
}

func (h *KofAssertions) ID() string    { return "kof-assertions" }
func (h *KofAssertions) Event() string { return EventSearch }

// kofBaseURL points at kof serve on localhost. KOF_PORT (with the pre-rename
// KEEP_PORT honored as a fallback) mirrors what the kof MCP reads, so both
// find the same instance.
func kofBaseURL() string {
	port := os.Getenv("KOF_PORT")
	if port == "" {
		port = os.Getenv("KEEP_PORT")
	}
	if port == "" {
		port = "7431"
	}
	return "http://127.0.0.1:" + port
}

// assertion is the subset of kof's API shape this hint renders.
type assertion struct {
	ID        string `json:"id"`
	Subject   string `json:"subject"`
	Statement string `json:"statement"`
	Status    string `json:"status"`
	Pins      []struct {
		File      string `json:"file"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	} `json:"pins"`
}

func (h *KofAssertions) Check(in Input) *Advice {
	subject := subjectFor(in)
	if subject == "" {
		return nil
	}
	if h.cfg.HintRepoExcludedTail(h.ID(), strings.ToLower(strings.TrimSpace(in.Repo))) {
		return nil
	}
	found := queryKof(h.base, repoPrefix(subject))
	relevant := rank(subject, found)
	if len(relevant) == 0 {
		return nil
	}
	fresh := seen.filter(in.SessionID, relevant)
	if len(fresh) == 0 {
		return nil
	}
	return &Advice{Hint: h.ID(), Text: render(subject, fresh)}
}

// subjectFor derives the subject of what the search actually looked at. It
// requires at least one path segment below the repo, so a search whose hits
// sit at the repo root does not drag in every assertion in a monorepo.
func subjectFor(in Input) string {
	repo := strings.ToLower(strings.TrimSpace(in.Repo))
	if repo == "" || len(in.Paths) == 0 {
		return ""
	}
	dir := commonDir(in.Paths)
	if dir == "" {
		return ""
	}
	return "repo:" + repo + "/" + dir
}

// repoPrefix reduces a hit subject to the repo it belongs to. Assertions are
// labelled at whatever depth the session that wrote them chose — usually the
// component (`.../services/csl`), which is shallower than the directory a
// search returns hits in (`.../services/csl/internal/semantic`). Since kof's
// subject filter matches by prefix, querying the deep hit subject would find
// nothing. Query the repo instead and let rank do the narrowing.
func repoPrefix(subject string) string {
	rest, ok := strings.CutPrefix(subject, "repo:")
	if !ok {
		return subject
	}
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		return subject
	}
	return "repo:" + parts[0] + "/" + parts[1] + "/"
}

// rank keeps assertions that overlap the searched location and orders them by
// how closely. Overlap is counted in path segments below the repo: an
// assertion on `services/csl` scores 2 against a hit in
// `services/csl/internal/semantic`, while one on `services/keeper-of-facts` scores 1 and
// is dropped for sharing only the generic `services` segment.
func rank(hitSubject string, as []assertion) []assertion {
	want := segmentsBelowRepo(hitSubject)
	if len(want) == 0 {
		return nil
	}
	type scored struct {
		a assertion
		n int
	}
	var out []scored
	for _, a := range as {
		n := commonSegments(want, segmentsBelowRepo(strings.ToLower(a.Subject)))
		if n < 2 && len(want) > 1 {
			continue // shares only a generic top segment like "services"
		}
		if n == 0 {
			continue
		}
		out = append(out, scored{a: a, n: n})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].n > out[j].n })
	if len(out) > maxAssertions {
		out = out[:maxAssertions]
	}
	res := make([]assertion, 0, len(out))
	for _, s := range out {
		res = append(res, s.a)
	}
	return res
}

func segmentsBelowRepo(subject string) []string {
	rest, ok := strings.CutPrefix(subject, "repo:")
	if !ok {
		return nil
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 3 {
		return nil
	}
	return parts[2:]
}

func commonSegments(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// commonDir returns the deepest directory prefix shared by every path, or ""
// when the hits span the repo root. Sharing a directory is what makes the
// assertions relevant to what the search actually found.
func commonDir(paths []string) string {
	var parts []string
	for i, p := range paths {
		segs := strings.Split(path.Dir(strings.TrimPrefix(p, "/")), "/")
		if len(segs) == 1 && (segs[0] == "." || segs[0] == "") {
			return ""
		}
		if i == 0 {
			parts = segs
			continue
		}
		n := 0
		for n < len(parts) && n < len(segs) && parts[n] == segs[n] {
			n++
		}
		parts = parts[:n]
		if len(parts) == 0 {
			return ""
		}
	}
	return strings.Join(parts, "/")
}

// queryKof asks kof serve for assertions under the subject prefix.
// Retracted ones are dropped: they are a record that a claim was withdrawn,
// not advice. Any failure returns nothing — kof being down must not produce
// noise.
func queryKof(base, subject string) []assertion {
	client := &http.Client{Timeout: kofTimeout}
	url := base + "/api/assertions?subject=" + urlEscape(subject)
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var body struct {
		Assertions []assertion `json:"assertions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil
	}
	var out []assertion
	for _, a := range body.Assertions {
		if a.Status == "retracted" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// render writes the advice block. Stale assertions are included and marked:
// a claim whose pinned code has since moved is the most useful thing kof can
// say about code being read right now, so suppressing it would waste the
// mechanism's best signal.
func render(subject string, as []assertion) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d stored assertion(s) for %s — prior sessions derived these; treat stale ones as needing re-verification.\n", len(as), subject)
	for _, a := range as {
		status := a.Status
		if status == "" {
			status = "fresh"
		}
		fmt.Fprintf(&b, "  [%s] %s\n", status, a.Statement)
		if len(a.Pins) > 0 {
			p := a.Pins[0]
			fmt.Fprintf(&b, "          %s:%d-%d (kof_get id=%s)\n", p.File, p.StartLine, p.EndLine, a.ID)
		}
	}
	b.WriteString("  Correct one that proves wrong with kof_retract rather than working around it.")
	return b.String()
}

func urlEscape(s string) string {
	return strings.NewReplacer(":", "%3A", "/", "%2F", " ", "%20").Replace(s)
}
