package guard

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestNetworkURL: a URL git reaches across the network is trusted; a
// folder on this machine, file:// and a remote helper are not.
func TestNetworkURL(t *testing.T) {
	for u, want := range map[string]bool{
		"https://github.com/o/r.git":    true,
		"ssh://git@host/o/r.git":        true,
		"git://host/r.git":              true,
		"git+ssh://host/r":              true,
		"git@github.com:o/r.git":        true,
		"host:r.git":                    true,
		"[git@host:22]:r.git":           true,
		"file:///srv/r.git":             false,
		"FILE:///srv/r.git":             false,
		"/srv/r.git":                    false,
		"../b":                          false,
		"b":                             false,
		"./host:r":                      false,
		"dir/host:r":                    false,
		"ext::sh -c touch% /tmp/x":      false,
		"persistent-https::https://h/r": false,
		"":                              false,
	} {
		if got := networkURL(u); got != want {
			t.Errorf("networkURL(%q) = %v, want %v", u, got, want)
		}
	}
}

// TestTrustedGlobs: the remote branches of a network remote are trusted
// unless a remote belt does not trust may fill refs there too.
func TestTrustedGlobs(t *testing.T) {
	origin := []string{"refs/remotes/origin/*", "refs/prefetch/remotes/origin/*"}
	tests := []struct {
		name    string
		entries []string
		want    []string
	}{
		{"network remote", []string{"remote.origin.url\nhttps://h/r", "remote.origin.fetch\n+refs/heads/*:refs/remotes/origin/*"}, origin},
		{"local remote", []string{"remote.origin.url\n../up.git"}, nil},
		{"no URL", []string{"remote.origin.fetch\n+refs/heads/*:refs/remotes/origin/*"}, nil},
		{"push URL in a folder", []string{"remote.origin.url\nhttps://h/r", "remote.origin.pushurl\n/srv/r.git"}, nil},
		{"second URL in a folder", []string{"remote.origin.url\nhttps://h/r", "remote.origin.url\n/srv/r.git"}, nil},
		{"insteadOf to a folder", []string{"remote.origin.url\nhttps://h/r", "url./srv/.insteadof\nhttps://h/"}, nil},
		{"pushInsteadOf to a folder", []string{"remote.origin.url\nhttps://h/r", "url./srv/.pushinsteadof\nhttps://"}, nil},
		{"insteadOf across the network", []string{"remote.origin.url\ngh:r", "url.https://h/.insteadof\ngh:"}, origin},
		{"rule that does not match", []string{"remote.origin.url\nhttps://h/r", "url./srv/.insteadof\nssh://"}, origin},
		{"local remote fills origin's branches", []string{"remote.origin.url\nhttps://h/r",
			"remote.loc.url\n../b", "remote.loc.fetch\n+refs/heads/*:refs/remotes/origin/*"}, nil},
		{"local remote fills every ref", []string{"remote.origin.url\nhttps://h/r",
			"remote.loc.url\n../b", "remote.loc.fetch\n+refs/*:refs/*"}, nil},
		{"local remote outside refs", []string{"remote.origin.url\nhttps://h/r",
			"remote.loc.url\n../b", "remote.loc.fetch\nrefs/heads/main:FETCH_HEAD"}, nil},
		{"local remote nested in origin's name", []string{"remote.origin.url\nhttps://h/r",
			"remote.origin/x.url\n../b"}, nil},
		{"local remote beside origin", []string{"remote.origin.url\nhttps://h/r", "remote.sib.url\n../b",
			"remote.sib.fetch\n+refs/heads/*:refs/remotes/sib/*", "remote.originx.url\n../c"}, origin},
		{"name with a glob", []string{"remote.o*.url\nhttps://h/r"}, nil},
		{"name with a dot", []string{"remote.my.origin.url\nhttps://h/r"},
			[]string{"refs/remotes/my.origin/*", "refs/prefetch/remotes/my.origin/*"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c remoteConfig
			for _, e := range tt.entries {
				key, val, ok := strings.Cut(e, "\n")
				c.add(key, val, ok)
			}
			if got := c.trustedGlobs(c.overNetwork); !slices.Equal(got, tt.want) {
				t.Errorf("trustedGlobs = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFileDiscardNestedTrust: a repository a delete takes whole passes
// only on the trust list. The first three rows are the losses earlier
// rounds let through, each with a real `rm -rf .build` from the clone: a
// worktree's staged change, a commit pushed into a mirror the drop takes,
// and remote branches of a remote the drop takes.
func TestFileDiscardNestedTrust(t *testing.T) {
	inBuild := func(rel string, change func(r discardRepos, dir string)) func(r discardRepos) {
		return func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", ".build/\n")
			dir := filepath.Join(r.clone, ".build", rel)
			r.git(r.clone, "clone", "-q", upstreamPath(r), dir)
			overNetwork(r, dir)
			if change != nil {
				change(r, dir)
			}
		}
	}
	lost := func(rel string) []string { return []string{"/.build/" + rel + "`", "is on no remote branch"} }
	runRepoRows(t, []repoRow{
		{"staged change in an outside worktree", inBuild("m", func(r discardRepos, m string) {
			wt := filepath.Join(filepath.Dir(r.clone), "wt")
			r.git(m, "worktree", "add", "-q", "--detach", wt)
			r.write(wt, "a.txt", "staged\n")
			r.git(wt, "add", "a.txt")
			r.write(wt, "a.txt", "a\n")
		}), "rm -rf .build", []string{"/.build/m`", "/wt` still exists", "/.build/m worktree remove "}},
		{"commit pushed into a mirror the drop takes", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", ".build/\n")
			up, b := filepath.Join(r.clone, ".build", "up.git"), filepath.Join(r.clone, ".build", "b")
			r.git(r.clone, "clone", "-q", "--mirror", upstreamPath(r), up)
			overNetwork(r, up)
			r.git(r.clone, "clone", "-q", up, b)
			commitFile(r, b, "x.txt")
			r.git(b, "push", "-q", "origin", "main")
		}, "rm -rf .build", []string{"is on no remote branch"}},
		{"remote branches of a remote the drop takes", inBuild("a", func(r discardRepos, a string) {
			r.git(a, "checkout", "-q", "-b", "feat")
			commitFile(r, a, "x.txt")
			r.git(a, "clone", "-q", ".", "../b")
			r.git(a, "remote", "add", "sib", "../b")
			r.git(a, "fetch", "-q", "sib")
		}), "rm -rf .build", lost("a")},
		// Each condition of the trust list, held and broken.
		{"clean clone of a network remote", inBuild("c", nil), "rm -rf .build", nil},
		{"pushed work in a clone of a network remote", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", ".build/\n")
			c := filepath.Join(r.clone, ".build", "c")
			r.git(r.clone, "clone", "-q", upstreamPath(r), c)
			r.git(c, "checkout", "-q", "-b", "topic")
			commitFile(r, c, "n.txt")
			r.git(c, "push", "-q", "origin", "topic")
			overNetwork(r, c)
		}, "rm -rf .build", nil},
		{"push URL in a local folder", inBuild("c", func(r discardRepos, c string) {
			r.git(c, "remote", "set-url", "--push", "origin", upstreamPath(r))
		}), "rm -rf .build", lost("c")},
		{"insteadOf turns the URL into a folder", inBuild("c", func(r discardRepos, c string) {
			r.git(c, "config", "url."+filepath.Dir(r.clone)+"/.insteadOf", "https://example.invalid/")
		}), "rm -rf .build", lost("c")},
		{"file URL", inBuild("c", func(r discardRepos, c string) {
			r.git(c, "remote", "set-url", "origin", "file://"+upstreamPath(r))
		}), "rm -rf .build", lost("c")},
		{"local remote fills origin's branches", inBuild("c", func(r discardRepos, c string) {
			loc := filepath.Join(filepath.Dir(r.clone), "loc.git")
			r.git(c, "init", "-q", "--bare", loc)
			r.git(c, "checkout", "-q", "-b", "side")
			commitFile(r, c, "s.txt")
			r.git(c, "push", "-q", loc, "side")
			r.git(c, "checkout", "-q", "main")
			r.git(c, "branch", "-q", "-D", "side")
			r.git(c, "remote", "add", "loc", loc)
			r.git(c, "config", "remote.loc.fetch", "+refs/heads/*:refs/remotes/origin/*")
			r.git(c, "fetch", "-q", "loc")
		}), "rm -rf .build", lost("c")},
		// SwiftPM's cache passes whole when its origin lies across the
		// network, and a checkout counts the cache's refs.
		{"SwiftPM cache and checkout", swiftRow(func(r discardRepos, mirror, checkout string) {
			overNetwork(r, mirror)
		}), "rm -rf .build", nil},
		{"SourceKit-LSP's index build", swiftRowIn(".build/index-build", func(r discardRepos, mirror, checkout string) {
			overNetwork(r, mirror)
		}), "rm -rf .build", nil},
		{"SwiftPM layout without its state file", swiftRow(func(r discardRepos, mirror, checkout string) {
			overNetwork(r, mirror)
			if err := os.Remove(filepath.Join(r.clone, ".build", "workspace-state.json")); err != nil {
				r.t.Fatal(err)
			}
		}), "rm -rf .build", lost("checkouts/dep")},
		{"SwiftPM checkout of a tag off every branch", func(r discardRepos) {
			offBranchTag(r)
			swiftRow(func(r discardRepos, mirror, checkout string) {
				r.git(checkout, "checkout", "-q", "--detach", "v2")
				overNetwork(r, mirror)
			})(r)
		}, "rm -rf .build", nil},
		{"SwiftPM cache of a local folder", swiftRow(nil), "rm -rf .build", lost("checkouts/dep")},
		{"SwiftPM cache of a local folder, alone", swiftRow(nil), "rm -rf .build/repositories",
			lost("repositories/dep-1")},
		{"SwiftPM cache with a linked worktree", swiftRow(func(r discardRepos, mirror, checkout string) {
			overNetwork(r, mirror)
			r.git(mirror, "worktree", "add", "-q", "--detach", filepath.Join(filepath.Dir(r.clone), "wt"), "v1")
		}), "rm -rf .build/repositories", []string{"/.build/repositories/dep-1`", "/wt` still exists"}},
		{"SwiftPM checkout with a local commit", swiftRow(func(r discardRepos, mirror, checkout string) {
			overNetwork(r, mirror)
			r.git(checkout, "checkout", "-q", "-b", "mine")
			commitFile(r, checkout, "n.txt")
		}), "rm -rf .build", lost("checkouts/dep")},
		{"SwiftPM checkout of another folder's cache", swiftRow(func(r discardRepos, mirror, checkout string) {
			overNetwork(r, mirror)
			other := filepath.Join(filepath.Dir(r.clone), "other", ".build", "repositories", "dep-1")
			r.git(r.clone, "clone", "-q", "--mirror", upstreamPath(r), other)
			overNetwork(r, other)
			r.git(checkout, "remote", "set-url", "origin", other)
		}), "rm -rf .build", lost("checkouts/dep")},
		{"SwiftPM checkout whose cache is not a mirror", swiftRow(func(r discardRepos, mirror, checkout string) {
			overNetwork(r, mirror)
			r.git(mirror, "config", "remote.origin.mirror", "false")
		}), "rm -rf .build", lost("checkouts/dep")},
		{"SwiftPM cache that is not a mirror, alone", swiftRow(func(r discardRepos, mirror, checkout string) {
			overNetwork(r, mirror)
			r.git(mirror, "config", "remote.origin.mirror", "false")
		}), "rm -rf .build/repositories", lost("repositories/dep-1")},
	})
}

// swiftRow sets up SwiftPM's layout for one package in .build
// (swiftRowIn).
func swiftRow(change func(r discardRepos, mirror, checkout string)) func(r discardRepos) {
	return swiftRowIn(".build", change)
}

// swiftRowIn sets up SwiftPM's layout for one package in the scratch
// folder at scratch (swiftLayout) with the checkout on tag v1, then runs
// change on the mirror and the checkout, if any.
func swiftRowIn(scratch string, change func(r discardRepos, mirror, checkout string)) func(r discardRepos) {
	return func(r discardRepos) {
		mirror, checkout := swiftLayout(r, "files", scratch)
		r.git(checkout, "checkout", "-q", "--detach", "v1")
		if change != nil {
			change(r, mirror, checkout)
		}
	}
}

// TestFileDiscardFdFeeder: fd's printed paths are read like find's, so an
// fd belt reads feeds xargs and $( ) as find does, and an option that
// prints more than paths or one belt does not read denies.
func TestFileDiscardFdFeeder(t *testing.T) {
	dirty := func(r discardRepos) {
		r.write(r.clone, ".git/info/exclude", "*.pyc\n")
		r.local(map[string]string{"x.pyc": "p", "d/x.pyc": "p", "a.txt": "mine\n"})
	}
	runRepoRows(t, []repoRow{
		{"fd into xargs over ignored files", dirty, "fd -I -e pyc | xargs rm", nil},
		{"fd into an xargs script", dirty, `fd -e pyc | xargs -n1 sh -c 'rm "$0"'`, nil},
		{"fd -0 into xargs -0", dirty, "fd -I -e pyc -0 | xargs -0 rm -f", nil},
		{"fd in a substitution", dirty, "rm $(fd -I -e pyc)", nil},
		{"fdfind into xargs", dirty, "fdfind -I -e pyc | xargs rm", nil},
		{"fd into xargs over a dirty file", dirty, "fd -e txt | xargs rm", []string{"`a.txt`"}},
		{"fd into an xargs script over a dirty file", dirty, `fd a.txt | xargs -n1 sh -c 'rm "$0"'`,
			[]string{"`a.txt`"}},
		{"fd in a substitution over a dirty file", dirty, "rm $(fd -e txt)", []string{"`a.txt`"}},
		{"fd -H into git data", dirty, "fd -H -I | xargs rm -f", []string{"git's own data"}},
		{"fd -a", dirty, "fd -a -I -e pyc | xargs rm", []string{"fd -a prints more than paths"}},
		{"fd --format", dirty, "fd -I -e pyc --format '{//}' | xargs rm -rf",
			[]string{"fd --format prints more than paths"}},
		{"fd -l", dirty, "fd -l -I -e pyc | xargs rm", []string{"fd --list-details prints more than paths"}},
		{"fd --color always", dirty, "fd -c always -I -e pyc | xargs rm", []string{"fd --color always"}},
		{"fd that runs a command", dirty, "fd -I -e pyc -x echo | xargs rm", []string{"fd -x prints what"}},
		{"fd with an unknown option", dirty, "fd --frobnicate -e pyc | xargs rm", []string{"fd's --frobnicate"}},
		{"fd -L", dirty, "fd -L -I -e pyc | xargs rm", []string{"fd -L follows symlinks"}},
	})
}
