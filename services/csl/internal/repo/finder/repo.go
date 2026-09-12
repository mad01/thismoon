package finder

import "github.com/mad01/thismoon/services/csl/internal/repo/catalogspec"

// Repo represents a discovered git repository.
type Repo struct {
	Name   string // org/repo extracted from remote URL
	Path   string // absolute filesystem path
	Remote string // full origin remote URL (e.g. "git@git.example.com:team/service.git")
	Host   string // extracted hostname (e.g. "github.com", "git.example.com")
	// Catalog is the Component the repo's root catalog descriptor declares
	// (catalog-info.yaml or service-info.yaml), nil when it has none. It is
	// read at discovery so every surface can match on owner and system
	// without a second walk.
	Catalog *catalogspec.Component
}

// DropKind names the rule that removed a repo from discovery. It is what a
// caller counts by; Dropped.Reason renders it in words, so a report never has
// to parse the sentence back apart.
type DropKind string

// The rules that drop a discovered repo.
const (
	// DropHost is the index.hosts allowlist rejecting a repo's remote: the
	// host is not listed, or the remote has no host to match (a local path).
	DropHost DropKind = "host"
	// DropNoRemote is that same allowlist rejecting a repo with no origin
	// remote at all, and so nothing to match against.
	DropNoRemote DropKind = "no-remote"
	// DropExcluded is the hooks.post_merge.exclude list naming a repo. The
	// walk never produces it — the config layer applies that list — but the
	// kind lives here so every drop speaks one vocabulary.
	DropExcluded DropKind = "excluded"
)

// Reason texts for the drop kinds whose wording is fixed; DropHost names the
// host through HostNotAllowedReason.
const (
	ReasonNoRemote = "no remote (index.hosts is set)"
	ReasonExcluded = "excluded by hooks.post_merge.exclude"
)

// Dropped is a repository the walk found and a filter removed: the repo as
// discovered and the rule that removed it.
type Dropped struct {
	Repo Repo
	Kind DropKind
}

// Reason names the setting that removed the repo, in words a user can act
// on. It is derived from Kind and the repo rather than stored, so a drop can
// never carry a sentence that disagrees with the rule it counts under.
func (d Dropped) Reason() string {
	switch d.Kind {
	case DropHost:
		if d.Repo.Host == "" {
			return "remote " + d.Repo.Remote + " has no host to match against index.hosts"
		}
		return HostNotAllowedReason(d.Repo.Host)
	case DropNoRemote:
		return ReasonNoRemote
	case DropExcluded:
		return ReasonExcluded
	}
	return string(d.Kind)
}

// HostNotAllowedReason is the reason text for a repo whose host is missing
// from the index.hosts allowlist.
func HostNotAllowedReason(host string) string {
	return "host " + host + " not in index.hosts"
}
