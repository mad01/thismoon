package cslignore

import (
	"maps"
	"strings"
)

// skipDirs are the directory names csl's walk never enters, on top of every
// hidden directory outside the allowlist.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	"build":        true,
	"dist":         true,
	"target":       true,
}

// allowedHiddenDirs are the hidden directory names the walk enters by
// default: CI workflows and agent configuration a developer searches for,
// never a cache, a virtualenv, or build output. A machine adds names with
// csl's index.allow_hidden_dirs config key; nothing removes one, and .git
// never joins.
var allowedHiddenDirs = map[string]bool{
	".github": true,
	".claude": true,
}

// HiddenAllowlist merges the built-in allowed hidden directories with the
// configured extras. .git is never entered whatever the config says.
func HiddenAllowlist(extra []string) map[string]bool {
	allow := make(map[string]bool, len(allowedHiddenDirs)+len(extra))
	maps.Copy(allow, allowedHiddenDirs)
	for _, name := range extra {
		allow[name] = true
	}
	delete(allow, ".git")
	return allow
}

// SkipDir reports whether csl's walk leaves out a directory named base: one
// of the generated trees, or a hidden directory allow does not name. A nil
// allow names none, so every hidden directory counts as left out.
func SkipDir(base string, allow map[string]bool) bool {
	return skipDirs[base] || (strings.HasPrefix(base, ".") && !allow[base])
}
