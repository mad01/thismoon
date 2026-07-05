package server

import (
	"time"

	"github.com/mad01/thismoon/services/pr/internal/github"
)

// prDetailJSON is the shape served at GET /api/pr/{host}/{owner}/{repo}/{number}.
// The detail page is client-side rendered (ADR-0017): the frontend (detail.js)
// computes display strings (relative ages, the checks summary) and builds the
// DOM. The two heavy content transforms — the GitHub-flavoured markdown body and
// the unified diff — stay server-side and ship as pre-rendered HTML, the same
// way present delivers its already-compiled body as data rather than duplicating
// the renderer in the browser.
type prDetailJSON struct {
	Number       int          `json:"number"`
	Title        string       `json:"title"`
	Author       string       `json:"author"`
	HTMLURL      string       `json:"html_url"`
	Draft        bool         `json:"draft"`
	Additions    int          `json:"additions"`
	Deletions    int          `json:"deletions"`
	ChangedFiles int          `json:"changed_files"`
	HeadRef      string       `json:"head_ref"`
	BaseRef      string       `json:"base_ref"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	Repo         repoJSON     `json:"repo"`
	BodyHTML     string       `json:"body_html"`
	DiffHTML     string       `json:"diff_html"`
	Files        []fileJSON   `json:"files"`
	Reviews      []reviewJSON `json:"reviews"`
	Checks       *checksJSON  `json:"checks"`
}

type repoJSON struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	HTMLURL string `json:"html_url"`
}

type fileJSON struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type reviewJSON struct {
	Author      string    `json:"author"`
	State       string    `json:"state"`
	Body        string    `json:"body"`
	SubmittedAt time.Time `json:"submitted_at"`
}

type checksJSON struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Pending int `json:"pending"`
}

// buildChecks reduces a check suite to pass/fail/pending counts. The summary
// text those counts feed is composed client-side.
func buildChecks(suite *github.CheckSuite) *checksJSON {
	if suite == nil {
		return nil
	}
	cv := &checksJSON{Total: suite.TotalCount}
	for _, cr := range suite.CheckRuns {
		switch cr.Conclusion {
		case "success":
			cv.Passed++
		case "failure", "cancelled", "timed_out":
			cv.Failed++
		default:
			cv.Pending++
		}
	}
	return cv
}
