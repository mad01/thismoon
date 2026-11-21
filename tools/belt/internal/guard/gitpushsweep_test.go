package guard

import (
	"slices"
	"testing"
)

// TestGitPushMainSweepsByDir pins the pushes that update a default branch
// without naming it, by holeDirs letter. --mirror, --all, a prefix git reads
// as one and the matching refspec : are judged like `git push origin main`,
// so an exempt repo (E) still takes them when they plainly go to origin. A
// config override that picks the refspecs, or maps a refspec with no :dst,
// drops the exemption, as any -c does, and a word only a shell can resolve
// outside the refspecs is denied everywhere. The allow cases must keep
// passing.
func TestGitPushMainSweepsByDir(t *testing.T) {
	runByDir(t, []byDirCase{
		// --mirror and --all, with or without force, any unique prefix.
		{"git push --mirror", "GNU"},
		{"git push --mirror origin", "GNU"},
		{"git push --mir origin", "GNU"},
		{"git push --m origin", "GNU"},
		{"git push -f --mirror origin", "GNU"},
		{"git push --all origin", "GNU"},
		{"git push --all --force origin", "GNU"},
		{"git push --all", "GNU"},
		{"git push --al origin", "GNU"},
		{"git push --branches origin", "GNU"},
		{"git push --b origin", "GNU"},
		{"git push --a origin", "GNU"},
		{"git push --all --prune origin", "GNU"},
		{"git push origin --mirror", "GNU"},
		{"git push -o -- --mirror origin", "GNU"},
		{"git push --mirror upstream", "GENU"},
		// The matching refspec, in every place git reads one.
		{"git push origin :", "GNU"},
		{"git push origin +:", "GNU"},
		{"git push --prune origin :", "GNU"},
		{"git push origin -- :", "GNU"},
		{"git push --end-of-options origin :", "GNU"},
		{"git push origin ':'", "GNU"},
		// Globs stay unsure, denied wherever they run.
		{"git push origin 'refs/heads/*'", "GENU"},
		{"git push --prune origin 'refs/heads/*:refs/heads/*'", "GENU"},
		// Config that picks the refspecs of a push that names none.
		{"git -c push.default=matching push", "GENU"},
		{"git -c push.default=matching push origin", "GENU"},
		{"git -c push.default=upstream push", "GENU"},
		{"git -c PUSH.DEFAULT=matching push", "GENU"},
		{"git -c remote.origin.mirror=true push", "GENU"},
		{"git -c remote.origin.mirror=true push --tags origin", "GENU"},
		{"git -c remote.origin.push=refs/heads/main push", "GENU"},
		{"git -c remote.origin.push=: push origin", "GENU"},
		{"git -c branch.feature.merge=refs/heads/main push", "GENU"},
		{"git --config-env=push.default=V push", "GENU"},
		{"git --config-env push.default=V push", "GENU"},
		{`git -c "$K=matching" push`, "GENU"},
		{`GIT_CONFIG_PARAMETERS="'push.default'='matching'" git push`, "GENU"},
		{"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=push.default GIT_CONFIG_VALUE_0=matching git push", "GENU"},
		{"export GIT_CONFIG_GLOBAL=/tmp/c; git push", "GENU"},
		{"HOME=/tmp/h git push", "GENU"},
		{"git config push.default matching && git push", "GENU"},
		{"git config remote.origin.mirror true; git push origin", "GENU"},
		// An include may set any of those keys.
		{"git -c include.path=/tmp/c push", "GENU"},
		{"git -c INCLUDE.PATH=/tmp/c push origin", "GENU"},
		{"git -c includeIf.onbranch:feature.path=/tmp/c push", "GENU"},
		{"git --config-env=include.path=V push", "GENU"},
		// Config that maps a refspec with no :dst onto another branch.
		{"git -c remote.origin.push=+refs/heads/feature:refs/heads/main push origin feature", "GENU"},
		{"git -c remote.origin.push=feature:heads/master push origin feature", "GENU"},
		{"git -c remote.origin.push=feature:HEAD push origin feature", "GENU"},
		{"git -c 'remote.origin.push=refs/heads/*:refs/heads/*' push origin feature", "GENU"},
		{"git -c Remote.Origin.Push=feature:main push origin feature:feature HEAD", "GENU"},
		{"git --config-env=remote.origin.push=V push origin feature", "GENU"},
		{`git -c remote.origin.push="$V" push origin feature`, "GENU"},
		{"git -c push.default=upstream push origin feature", "GENU"},
		{"git -c push.default=tracking push origin feature", "GENU"},
		{"git -c branch.feature.merge=refs/heads/main push origin feature", "GENU"},
		{"git -c include.path=/tmp/c push origin feature", "GENU"},
		{"git -c includeIf.onbranch:feature.path=/tmp/c push origin feature", "GENU"},
		{"git config remote.origin.push +feature:main && git push origin feature", "GENU"},
		{"GIT_CONFIG_GLOBAL=/tmp/c git push origin feature", "GENU"},
		// A word only a shell can resolve outside the refspecs.
		{`git push "$X"`, "GENU"},
		{"git push $X", "GENU"},
		{"git push $X feature", "GENU"},
		{`git push "$(echo --mirror)" origin`, "GENU"},
		{"git push {--mirror,} origin", "GENU"},
		{"git push --mir* origin", "GENU"},
		{`git push -o "$X" origin feature`, "GENU"},
		{`git push --repo="$R" feature`, "GENU"},
		// A subcommand word only a shell can resolve may hide a push after it.
		{"git $X push origin main", "GENU"},
		{`git "$X" push origin feature`, "GENU"},
		{"git ${X:+-c} ${X:+k=v} push origin feature", "GENU"},
		{"git --no-pager $X push", "GENU"},
		{"git $X status", ""},
		// Any push word after it counts, a value included, as belt cannot
		// tell which word git takes as the subcommand.
		{"git $X log --grep push", "GENU"},
		// Pushes that name what they push keep passing.
		{"git push origin feature", ""},
		{"git push -u origin feature", ""},
		{"git push -fu origin feature", ""},
		{"git push --force-with-lease origin feature", ""},
		{"git push --no-mirror origin feature", ""},
		{"git push --follow-tags origin feature", ""},
		{"git push -o ci.skip origin feature", ""},
		{"git push origin -- feature", ""},
		{"git -c core.sshCommand=ssh push origin feature", ""},
		{"git -c remote.origin.push=refs/heads/main push origin feature", ""},
		{"git -c remote.origin.push=refs/heads/feature:refs/heads/feature push origin feature", ""},
		{"git -c remote.origin.push=+feature:main push origin feature:feature", ""},
		{"git -c remote.origin.push=+feature:main push --delete origin feature", ""},
		{"git -c include.path=/tmp/c push origin feature:feature", ""},
		{"git -c push.default=matching push origin feature", ""},
		{"git -c remote.origin.mirror=true push origin feature", ""},
		// A push that needs the current branch passes where belt can read it.
		{"git push --tags", "U"},
		{"git push --tags origin", "U"},
		{"git -c push.default=simple push", "U"},
		{"git -c core.sshCommand=ssh push", "U"},
		{"GIT_SSH_COMMAND=ssh git push", "U"},
		{`git push -u origin "$(git branch --show-current)"`, "U"},
	})
}

// TestGitPushMainSweepReasons pins what a sweep denial says: the form that
// sweeps, and in an exempt repo the plain origin form the exemption covers.
func TestGitPushMainSweepReasons(t *testing.T) {
	runHoleCases(t, []holeCase{
		{"mirror", "feature", "git push --mir origin", false, true, "--mirror may make this push"},
		{"all", "feature", "git push --al origin", false, true, "--all may make this push"},
		{"matching", "feature", "git push origin +:", false, true, `the matching refspec ":"`},
		{
			"config key",
			"feature",
			"git -c push.default=matching push",
			false,
			true,
			"the config override push.default=matching",
		},
		{
			"config remap",
			"feature",
			"git -c remote.origin.push=+feature:main push origin feature",
			false,
			true,
			"the config override remote.origin.push=+feature:main may make this push",
		},
		{
			"config change",
			"feature",
			"git config push.default matching && git push",
			false,
			true,
			"config the command sets or changes",
		},
		{
			"exempt mirror to another remote",
			"feature",
			"git push --mirror upstream",
			true,
			true,
			"`git push --mirror origin`",
		},
		{
			"exempt all with -c",
			"feature",
			"git -c push.default=simple push --all origin",
			true,
			true,
			"`git push --all origin`",
		},
		{"exempt mirror to origin", "feature", "git push --mirror origin", true, false, ""},
		{"exempt all on main", "main", "git push --all", true, false, ""},
		{"shell word", "feature", `git push "$X"`, true, true, `cannot tell what "$X" gives`},
	})
}

// TestParsePushArgsReadsLikeParseOptions pins the push reader against git's
// parse-options: unique long prefixes, short bundles, a value option that
// takes the next word even when it is --, and -- or --end-of-options ending
// the options.
func TestParsePushArgsReadsLikeParseOptions(t *testing.T) {
	tests := []struct {
		name, remote string
		args         []string
		refspecs     []string
		sweep        string
		shellWord    string
		repoFlag     bool
	}{
		{"prefix of mirror", "origin", []string{"--mirr", "origin"}, nil, "--mirror", "", false},
		{"branches alias", "origin", []string{"--bra", "origin"}, nil, "--all", "", false},
		{
			"negation sets nothing",
			"origin",
			[]string{"--no-mirror", "--no-all", "origin", "feature"},
			[]string{"feature"},
			"",
			"",
			false,
		},
		{
			"value option takes --",
			"origin",
			[]string{"--repo", "--", "origin", "feature"},
			[]string{"feature"},
			"",
			"",
			true,
		},
		{"prefix of repo takes a value", "feature", []string{"--rep", "origin", "feature"}, nil, "", "", true},
		{"repo = form", "origin", []string{"--rep=x", "origin", "a"}, []string{"a"}, "", "", true},
		{
			"bundle ending in o takes the next word",
			"origin",
			[]string{"-fuo", "--mirror", "origin", "a"},
			[]string{"a"},
			"",
			"",
			false,
		},
		{"bundle with o inline", "origin", []string{"-fomain", "origin", "a"}, []string{"a"}, "", "", false},
		{
			"double dash ends options",
			"origin",
			[]string{"origin", "--", "--mirror"},
			[]string{"--mirror"},
			"",
			"",
			false,
		},
		{
			"end-of-options ends options",
			"origin",
			[]string{"--end-of-options", "origin", ":"},
			[]string{":"},
			`the matching refspec ":"`,
			"",
			false,
		},
		{"shell word in a value", "origin", []string{"-o", "$X", "origin"}, nil, "", "$X", false},
		{"shell word as remote", `"$R"`, []string{`"$R"`, "feature"}, []string{"feature"}, "", `"$R"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parsePushArgs(gitInvocation{sub: "push", args: tt.args})
			if p.remote != tt.remote || !slices.Equal(p.refspecs, tt.refspecs) {
				t.Errorf("remote %q refspecs %q, want %q %q", p.remote, p.refspecs, tt.remote, tt.refspecs)
			}
			if p.sweep != tt.sweep || p.shellWord != tt.shellWord || p.repoFlag != tt.repoFlag {
				t.Errorf(
					"sweep %q shellWord %q repoFlag %v, want %q %q %v",
					p.sweep, p.shellWord, p.repoFlag, tt.sweep, tt.shellWord, tt.repoFlag,
				)
			}
		})
	}
}
