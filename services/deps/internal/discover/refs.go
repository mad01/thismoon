package discover

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// advertisementType is the content type of a smart-HTTP ref advertisement.
// A dumb-HTTP server or a login page answers with something else.
const advertisementType = "application/x-git-upload-pack-advertisement"

// maxAdvertisement caps one ref advertisement. tj-actions/changed-files, with
// thousands of tags and branches, sends about 200 KB.
const maxAdvertisement = 16 << 20

// advertisementClient fetches ref advertisements. Its timeout bounds a scan
// that github.com stops answering.
var advertisementClient = &http.Client{Timeout: 30 * time.Second}

// refCache memoizes ref advertisements by URL for one discovery walk, the
// error included, so each action repo is fetched at most once per scan however
// many repos and workflows pin it.
type refCache map[string]advertised

// advertised is one fetched advertisement: ref name to commit, or the error
// that fetching it gave.
type advertised struct {
	refs map[string]string
	err  error
}

// advertisement returns the refs advertised at url, fetching them unless this
// walk already has.
func (c refCache) advertisement(url string) (map[string]string, error) {
	if a, ok := c[url]; ok {
		return a.refs, a.err
	}
	refs, err := fetchAdvertisement(url)
	c[url] = advertised{refs: refs, err: err}
	return refs, err
}

// fetchAdvertisement GETs a smart-HTTP ref advertisement, the same list `git
// ls-remote` prints, and parses it. github.com answers 401 for a repo that is
// missing or private.
func fetchAdvertisement(url string) (map[string]string, error) {
	res, err := advertisementClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return nil, errors.New("ref list answered 401: the repo is missing or private")
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("ref list answered %d", res.StatusCode)
	case !strings.HasPrefix(res.Header.Get("Content-Type"), advertisementType):
		return nil, fmt.Errorf(
			"ref list content type is %q, want %s",
			res.Header.Get("Content-Type"), advertisementType,
		)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxAdvertisement+1))
	if err != nil {
		return nil, fmt.Errorf("read ref list: %w", err)
	}
	if len(raw) > maxAdvertisement {
		return nil, fmt.Errorf("ref list is larger than %d bytes", maxAdvertisement)
	}
	return parseAdvertisement(raw)
}

// parseAdvertisement reads a git-upload-pack ref advertisement in pkt-line
// form: a "# service=" line, then one "<sha> <ref>" line per ref, the first
// carrying the capabilities after a NUL. An annotated tag is followed by a
// "<ref>^{}" line with the commit it points to, and that commit replaces the
// tag object. It returns ref name to commit.
func parseAdvertisement(raw []byte) (map[string]string, error) {
	refs := map[string]string{}
	peeled := map[string]string{}
	for len(raw) > 0 {
		if len(raw) < 4 {
			return nil, errors.New("ref list ends inside a pkt-line length")
		}
		n, err := strconv.ParseUint(string(raw[:4]), 16, 16)
		if err != nil {
			return nil, fmt.Errorf("ref list has a bad pkt-line length %q", raw[:4])
		}
		if n == 0 { // flush-pkt
			raw = raw[4:]
			continue
		}
		if n < 4 || int(n) > len(raw) {
			return nil, fmt.Errorf(
				"ref list has a pkt-line of length %d with %d bytes left",
				n,
				len(raw),
			)
		}
		line := string(bytes.TrimSuffix(raw[4:n], []byte("\n")))
		raw = raw[n:]
		line, _, _ = strings.Cut(line, "\x00")
		sha, name, ok := strings.Cut(line, " ")
		if !ok || !isCommitSHA(sha) {
			continue // the "# service=" line
		}
		if tag, ok := strings.CutSuffix(name, "^{}"); ok {
			peeled[tag] = sha
			continue
		}
		refs[name] = sha
	}
	for tag, sha := range peeled {
		refs[tag] = sha
	}
	return refs, nil
}

// resolveRef returns the commit an action ref names and whether it is a tag or
// a branch. A tag wins over a branch of the same name, as in git rev-parse.
// A ref spelled in full (refs/tags/v1) is looked up as given.
func resolveRef(refs map[string]string, ref string) (commit, kind string, ok bool) {
	for _, c := range []struct{ prefix, kind string }{
		{"refs/tags/", "tag"},
		{"refs/heads/", "branch"},
	} {
		name := ref
		if !strings.HasPrefix(ref, "refs/") {
			name = c.prefix + ref
		}
		if sha, ok := refs[name]; ok && strings.HasPrefix(name, c.prefix) {
			return strings.ToLower(sha), c.kind, true
		}
	}
	return "", "", false
}
