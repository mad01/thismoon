package finder

import (
	"fmt"
	"strings"
)

// FileURL builds a browser URL to a file (optionally at a line) on the repo's
// git host. It targets the default branch via the "HEAD" ref, which GitHub and
// GitHub Enterprise both resolve to the repo's default branch:
//
//	https://{Host}/{Name}/blob/HEAD/{file}#L{line}
//
// It returns "" when the repo has no host or name (e.g. a repo with no remote),
// so callers can omit the link rather than render a broken one. A non-positive
// line omits the "#L" fragment.
func FileURL(r Repo, file string, line int) string {
	if r.Host == "" || r.Name == "" {
		return ""
	}
	file = strings.TrimPrefix(file, "/")
	url := fmt.Sprintf("https://%s/%s/blob/HEAD/%s", r.Host, r.Name, file)
	if line > 0 {
		url += fmt.Sprintf("#L%d", line)
	}
	return url
}
