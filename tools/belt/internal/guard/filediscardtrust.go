package guard

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// This file holds the remotes file-discard trusts to hold the commits of a
// repository a delete takes whole: a remote git reaches across the network,
// read from git config, and SwiftPM's package cache (.build/repositories).
// Every other remote is a folder the same delete may take, so its refs do
// not count.

// gitRemote is one remote as git config names it: the URLs git fetches
// from and pushes to, the refs its fetch refspecs fill, and whether it is a
// mirror.
type gitRemote struct {
	name           string
	urls, pushURLs []string
	fetchDsts      []string
	mirror         bool
}

// urlRewrite is a url.<to>.insteadOf or pushInsteadOf rule: git reads a
// URL that starts with from as to and the rest.
type urlRewrite struct{ from, to string }

// remoteConfig is what git config says about a repository's remotes.
type remoteConfig struct {
	remotes  []*gitRemote
	rewrites []urlRewrite
}

// readRemotes reads the remotes of the repository git reads with at, from
// every config scope git reads there.
func readRemotes(ctx context.Context, repo string, at []string) (remoteConfig, error) {
	out, err := runProofGit(ctx, repo, slices.Concat(at, []string{"config", "--list", "-z"})...)
	if err != nil {
		return remoteConfig{}, fmt.Errorf("belt could not read its git config (%v)", err)
	}
	var c remoteConfig
	for _, f := range nulFields(out) {
		key, val, hasVal := strings.Cut(f, "\n")
		c.add(key, val, hasVal)
	}
	return c, nil
}

// add takes one config entry. git prints the section and the variable in
// lower case and the remote's name as written, which may hold dots; an
// entry with no value is a true bool.
func (c *remoteConfig) add(key, val string, hasVal bool) {
	dot := strings.LastIndexByte(key, '.')
	if dot < 0 {
		return
	}
	section, name, ok := strings.Cut(key[:dot], ".")
	if !ok {
		return
	}
	switch v := key[dot+1:]; {
	case section == "url" && (v == "insteadof" || v == "pushinsteadof"):
		c.rewrites = append(c.rewrites, urlRewrite{from: val, to: name})
	case section != "remote":
	case v == "url":
		c.remote(name).urls = append(c.remote(name).urls, val)
	case v == "pushurl":
		c.remote(name).pushURLs = append(c.remote(name).pushURLs, val)
	case v == "fetch":
		if dst, ok := fetchDst(val); ok {
			c.remote(name).fetchDsts = append(c.remote(name).fetchDsts, dst)
		}
	case v == "mirror":
		c.remote(name).mirror = !hasVal || gitTrue(val)
	}
}

// remote returns the remote called name, added when it is new.
func (c *remoteConfig) remote(name string) *gitRemote {
	for _, r := range c.remotes {
		if r.name == name {
			return r
		}
	}
	r := &gitRemote{name: name}
	c.remotes = append(c.remotes, r)
	return r
}

// fetchDst returns the ref a fetch refspec fills, false for one that fills
// none: a negative refspec, or one with no destination.
func fetchDst(spec string) (string, bool) {
	if strings.HasPrefix(spec, "^") {
		return "", false
	}
	_, dst, ok := strings.Cut(strings.TrimPrefix(spec, "+"), ":")
	return dst, ok && dst != ""
}

// gitTrue reports whether a config value is true as git reads a bool.
func gitTrue(v string) bool {
	switch strings.ToLower(v) {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}

// overNetwork reports whether git reaches the remote r across the network
// alone: it has a URL, and each URL it fetches from or pushes to is a
// network URL, as written and as each rewrite rule that matches turns it.
// A push updates the remote branches too, so a push URL in a local folder
// makes them untrustworthy.
func (c remoteConfig) overNetwork(r *gitRemote) bool {
	if len(r.urls) == 0 {
		return false
	}
	for _, u := range slices.Concat(r.urls, r.pushURLs) {
		if !networkURL(u) {
			return false
		}
		for _, w := range c.rewrites {
			if strings.HasPrefix(u, w.from) && !networkURL(w.to+u[len(w.from):]) {
				return false
			}
		}
	}
	return true
}

// networkURL reports whether git reaches the repository at u across the
// network: a URL whose scheme is not file, or the scp-like [user@]host:path,
// where a colon comes before any slash. A local path, a file:// URL and a
// remote helper's <transport>::<address> are not.
func networkURL(u string) bool {
	if i := schemeEnd(u); i > 0 {
		switch rest := u[i:]; {
		case strings.HasPrefix(rest, "::"):
			return false
		case strings.HasPrefix(rest, "://"):
			return !strings.EqualFold(u[:i], "file")
		}
	}
	colon, slash := strings.IndexByte(u, ':'), strings.IndexByte(u, '/')
	return colon > 0 && (slash < 0 || colon < slash)
}

// schemeEnd returns how many bytes at the start of u git reads as a URL
// scheme: a letter or digit, then letters, digits, +, - and dots.
func schemeEnd(u string) int {
	for i := 0; i < len(u); i++ {
		c := u[i]
		alnum := 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
		if !alnum && (i == 0 || c != '+' && c != '-' && c != '.') {
			return i
		}
	}
	return len(u)
}

// trackingRefs are the folders of refs git fills from the remote called
// name: its remote branches, and the ones git maintenance's prefetch task
// fills.
func trackingRefs(name string) []string {
	return []string{"refs/remotes/" + name + "/", "refs/prefetch/remotes/" + name + "/"}
}

// trustedGlobs returns the ref globs that hold only what a trusted remote
// held: the trackingRefs of each remote trusts accepts, unless a remote it
// does not accept may fill refs there too. A remote fills its own
// trackingRefs and the destination of each fetch refspec, which the
// prefetch task moves under refs/prefetch/. A remote whose name holds a
// glob character is not trusted.
func (c remoteConfig) trustedGlobs(trusts func(*gitRemote) bool) []string {
	var others, trusted []string
	var names []string
	for _, r := range c.remotes {
		if trusts(r) && !strings.ContainsAny(r.name, `*?[\`) {
			names = append(names, r.name)
			continue
		}
		others = append(others, trackingRefs(r.name)...)
		for _, dst := range r.fetchDsts {
			p := refPrefix(dst)
			others = append(others, p)
			if rest, ok := strings.CutPrefix(p, "refs/"); ok {
				others = append(others, "refs/prefetch/"+rest)
			}
		}
	}
	for _, name := range names {
		own := trackingRefs(name)
		if slices.ContainsFunc(others, func(p string) bool { return overlaps(p, own[0]) || overlaps(p, own[1]) }) {
			continue
		}
		for _, p := range own {
			trusted = append(trusted, p+"*")
		}
	}
	return trusted
}

// refPrefix returns the part of a refspec destination before its glob,
// "" for one outside refs/, which counts as one that may fill any ref.
func refPrefix(dst string) string {
	if !strings.HasPrefix(dst, "refs/") {
		return ""
	}
	if i := strings.IndexByte(dst, '*'); i >= 0 {
		return dst[:i]
	}
	return dst
}

// overlaps reports whether a ref may start with both prefixes.
func overlaps(a, b string) bool { return strings.HasPrefix(a, b) || strings.HasPrefix(b, a) }

// SwiftPM keeps a mirror of each package in the repositories folder of its
// scratch folder and checks each one out in checkouts with `git clone
// --shared` from that mirror, so the checkout's origin is the mirror's
// folder. The scratch folder is .build, .build/index-build for
// SourceKit-LSP's index build, or any --scratch-path; SwiftPM keeps its
// workspace-state.json in each.
const (
	swiftState        = "workspace-state.json"
	swiftRepositories = "repositories"
	swiftCheckouts    = "checkouts"
)

// inSwiftFolder reports whether the repository at repo lies directly in
// the folder sub of a SwiftPM scratch folder, one that holds swiftState.
func inSwiftFolder(repo, sub string) bool {
	parent := filepath.Dir(repo)
	return filepath.Base(parent) == sub && exists(filepath.Join(filepath.Dir(parent), swiftState))
}

// swiftCache reports whether the bare repository at repo, whose remotes c
// lists, is SwiftPM's cache of a package: a mirror directly in the
// repositories folder of a SwiftPM scratch folder, with no linked
// worktree, each remote of which is a mirror git reaches across the
// network. A cache holds only what its remotes held, so it goes whole.
func swiftCache(repo string, c remoteConfig) bool {
	if !inSwiftFolder(repo, swiftRepositories) || len(c.remotes) == 0 {
		return false
	}
	if trees, err := worktreeAdmins(repo); err != nil || len(trees) > 0 {
		return false
	}
	return !slices.ContainsFunc(c.remotes, func(r *gitRemote) bool { return !r.mirror || !c.overNetwork(r) })
}

// swiftOrigin returns the SwiftPM cache the remote r of the work tree at
// repo names, "" for none: repo lies in the checkouts folder of a SwiftPM
// scratch folder, r has one URL and no push URL, no rewrite rule matches
// it, and it names a repository in the same scratch folder's repositories
// that is a cache (swiftCache).
func swiftOrigin(ctx context.Context, repo string, c remoteConfig, r *gitRemote) string {
	if !inSwiftFolder(repo, swiftCheckouts) || len(r.urls) != 1 || len(r.pushURLs) > 0 {
		return ""
	}
	u := r.urls[0]
	if slices.ContainsFunc(c.rewrites, func(w urlRewrite) bool { return strings.HasPrefix(u, w.from) }) {
		return ""
	}
	dir := strings.TrimPrefix(u, "file://")
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repo, dir)
	}
	dir = physical(filepath.Clean(dir))
	cache := physical(filepath.Join(filepath.Dir(filepath.Dir(repo)), swiftRepositories))
	if filepath.Dir(dir) != cache || exists(filepath.Join(dir, ".git")) {
		return ""
	}
	mc, err := readRemotes(ctx, repo, []string{"--git-dir=" + dir})
	if err != nil || !swiftCache(dir, mc) {
		return ""
	}
	return dir
}

// remoteTrust is what holds a repository's commits for file-discard: the
// refs its trusted remotes fill (trustedGlobs), and a SwiftPM cache whose
// refs hold them too, "" for none.
type remoteTrust struct {
	globs []string
	cache string
}

// trustOf returns what belt trusts to hold the commits of the repository
// at repo, whose remotes c lists: each remote across the network, and for
// a SwiftPM checkout the cache its origin names.
func trustOf(ctx context.Context, repo string, bare bool, c remoteConfig) remoteTrust {
	var t remoteTrust
	var cacheRemote *gitRemote
	if !bare {
		for _, r := range c.remotes {
			if t.cache = swiftOrigin(ctx, repo, c, r); t.cache != "" {
				cacheRemote = r
				break
			}
		}
	}
	t.globs = c.trustedGlobs(func(r *gitRemote) bool { return r == cacheRemote || c.overNetwork(r) })
	return t
}

// cacheTips lists the objects the refs of the SwiftPM cache at cache point
// at.
func cacheTips(ctx context.Context, repo, cache string) ([]string, error) {
	out, err := runProofGit(ctx, repo, "--git-dir="+cache, "for-each-ref", "--format=%(objectname)")
	if err != nil {
		return nil, fmt.Errorf("belt could not read the refs of its SwiftPM cache %s (%v)", cache, err)
	}
	return strings.Fields(string(out)), nil
}
