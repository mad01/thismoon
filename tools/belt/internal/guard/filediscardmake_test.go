package guard

import (
	"path/filepath"
	"strings"
	"testing"
)

// The rows below run make commands through file-discard, as Check reads
// them: belt reads the makefile, and nothing runs.

// makefileText joins makefile lines; a line that starts with > is a recipe
// line, written with make's tab.
func makefileText(lines ...string) string {
	for i, l := range lines {
		if rest, ok := strings.CutPrefix(l, ">"); ok {
			lines[i] = "\t" + rest
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// TestFileDiscardMakeTargets covers the targets a make command runs, their
// prerequisites and the variables in their recipes, over a temp folder,
// which passes, and a folder in no work tree, which denies.
func TestFileDiscardMakeTargets(t *testing.T) {
	home := t.TempDir()
	tmp := tempFixture(t)
	g := newFileDiscardGuard(t, home)
	writeTree(t, home, map[string]string{
		"work/notes.txt": "n\n",
		"Makefile": makefileText(
			".PHONY: clean", "OUT = work", "TMPOUT := "+tmp+"/out", "LATE ?= work",
			"clean: sub", ">@rm -rf $(TMPOUT)",
			"sub:", ">-rm -rf ${OUT}/notes.txt",
			"tmp:", ">+rm -rf $(TMPOUT)",
			"var:", ">rm -rf $(NOPE)/x",
			"dollar:", ">rm -rf $$HOME/work",
			"canned:", ">$(call zap,work)",
			"define zap", "rm -rf $(1)", "endef",
			"shelled:", ">echo $(shell rm -rf work)",
			"loop:", ">$(foreach d,work,rm -rf $(d);)",
			"late:", ">rm -rf $(LATE)",
			"ifeq ($(OS),Windows_NT)", "DEL = del /q", "else", "DEL = rm -rf", "endif",
			"cond:", ">$(DEL) work",
			"ts: OUT = "+tmp+"/y", "ts:", ">rm -rf $(OUT)",
			"%.zap:", ">rm -rf $*",
			"inline: ; rm -rf work",
			"cont:", ">rm -rf \\", ">  work",
			"auto-work:", ">rm -rf $(@:auto-%=%)",
			"plus:", ">+rm -rf work",
		),
		"other.mk":         makefileText("go:", ">rm -rf work"),
		"g/GNUmakefile":    makefileText("x:", ">rm -rf ../work"),
		"g/Makefile":       makefileText("x:", ">echo fine"),
		"inc/Makefile":     makefileText("include more.mk", "x:", ">echo x"),
		"inc/more.mk":      makefileText("gone:", ">rm -rf ../work"),
		"one/Makefile":     makefileText(".ONESHELL:", "go:", ">cd "+tmp, ">rm -rf x"),
		"one/x/f.txt":      "f\n",
		"partial/Makefile": makefileText("$(eval $(call gen,x))", "y:", ">echo y"),
		"env/Makefile":     makefileText("OUT = "+tmp+"/y", "clean-out:", ">rm -rf $(OUT)"),
		"env/extra.mk":     makefileText("override OUT = ../work"),
		"pv/Makefile":      makefileText("OUT = "+tmp+"/y", "%-out: OUT = ../work", "clean-out:", ">rm -rf $(OUT)"),
		"prefix/Makefile":  ".RECIPEPREFIX = >\nclean:\n>rm -rf ../work\n",
		"second/Makefile": makefileText(".SECONDEXPANSION:", "clean: $$(DEP)", ">echo", "DEP = zap",
			"zap:", ">rm -rf ../work"),
		"py/Makefile": makefileText("SHELL := python3", ".SHELLFLAGS := -c", "clean:",
			">import shutil; shutil.rmtree('../work')"),
		"fish/Makefile": makefileText("SHELL := /usr/local/bin/fish", "clean:", ">echo hi"),
	})
	noTree := "which is in no git work tree"
	unknown := "belt cannot tell which path"
	rows := []fileDiscardRow{
		{"default goal and prerequisite", "~", "make", []string{"the make target `sub` in Makefile:8", noTree}, nil},
		{"named target", "~", "make tmp", nil, nil},
		{"unknown variable", "~", "make var", []string{unknown}, nil},
		{"double dollar", "~", "make dollar", []string{noTree}, nil},
		{"canned recipe", "~", "make canned", []string{noTree}, nil},
		{"shell function", "~", "make shelled", []string{noTree}, nil},
		{"function naming a delete", "~", "make loop", []string{"belt cannot read what `$(foreach"}, nil},
		{"command line variable", "~", "make sub OUT=" + tmp, nil, nil},
		{"environment fills ?=", "~", "LATE=" + tmp + "/late make late", nil, nil},
		{"?= default", "~", "make late", []string{noTree}, nil},
		{"conditional values", "~", "make cond", []string{noTree}, nil},
		{"target-specific variable", "~", "make ts", nil, nil},
		{"pattern rule", "~", "make work.zap", []string{noTree}, nil},
		{"inline recipe", "~", "make inline", []string{noTree}, nil},
		{"continued line", "~", "make cont", []string{noTree}, nil},
		{"substitution reference", "~", "make auto-work", []string{noTree}, nil},
		{"dry run", "~", "make -n sub", nil, nil},
		{"dry run runs + lines", "~", "make -n plus", []string{noTree}, nil},
		{"version", "~", "make --version", nil, nil},
		{"no such target", "~", "make nothere", nil, nil},
		{"makefile option", "~", "make -f other.mk go", []string{"other.mk:2", noTree}, nil},
		{"folder option", "~", "make -C g x", []string{"GNUmakefile", noTree}, nil},
		{"folder after cd", "~", "cd g && make x", []string{noTree}, nil},
		{"include", "~", "make -C inc gone", []string{"more.mk", noTree}, nil},
		{"oneshell lines apart", "~", "make -C one go", []string{noTree}, nil},
		{"partial makefile, missing target", "~", "make -C partial zz", []string{"belt cannot read the makefile line"}, nil},
		{"partial makefile, found target", "~", "make -C partial y", nil, nil},
		{"no makefile", "~/work", "make clean", nil, nil},
		{"behind sudo", "~", "sudo make sub", []string{noTree}, nil},
		{"variable target", "~", "make $T", []string{"belt cannot tell what `$T` asks make to run"}, nil},
		{"environment overrides", "~", "OUT=../work make -e -C env clean-out", []string{noTree}, nil},
		{"environment, no -e", "~", "OUT=../work make -C env clean-out", nil, nil},
		{"MAKEFLAGS holds -e", "~", "MAKEFLAGS=-e OUT=../work make -C env clean-out", []string{noTree}, nil},
		{"MAKEFLAGS letters", "~", "MAKEFLAGS=ke OUT=../work make -C env clean-out", []string{noTree}, nil},
		{"MAKEFLAGS without -e", "~", "MAKEFLAGS=-k OUT=../work make -C env clean-out", nil, nil},
		{"MAKEFILES", "~", "MAKEFILES=extra.mk make -C env clean-out", []string{noTree}, nil},
		{"pattern-specific variable", "~", "make -C pv clean-out", []string{noTree}, nil},
		{"recipe prefix", "~", "make -C prefix clean", []string{noTree}, nil},
		{"second expansion", "~", "make -C second clean", []string{noTree}, nil},
		{"python SHELL", "~", "make -C py clean", []string{"`clean` in Makefile:4", noTree}, nil},
		{"other SHELL", "~", "make -C fish clean", []string{"the SHELL `/usr/local/bin/fish`"}, nil},
	}
	runFileDiscardRows(t, g, home, rows)
}

// TestReadMakeArgs covers make's option forms.
func TestReadMakeArgs(t *testing.T) {
	words := func(ws ...string) []shWord {
		out := make([]shWord, len(ws))
		for i, w := range ws {
			out[i] = literalWord(w)
		}
		return out
	}
	tests := []struct {
		name    string
		args    []string
		dir     string
		files   []string
		targets []string
		dry     bool
	}{
		{"plain", []string{"a", "b"}, "/r", nil, []string{"a", "b"}, false},
		{"joined -C", []string{"-Csub", "a"}, "/r/sub", nil, []string{"a"}, false},
		{"long directory", []string{"--directory=sub", "a"}, "/r/sub", nil, []string{"a"}, false},
		{"jobs number", []string{"-j", "4", "a"}, "/r", nil, []string{"a"}, false},
		{"jobs then target", []string{"-j", "a"}, "/r", nil, []string{"a"}, false},
		{"long jobs number", []string{"--jobs", "4", "a"}, "/r", nil, []string{"a"}, false},
		{"cluster with file", []string{"-sf", "x.mk", "a"}, "/r", []string{"x.mk"}, []string{"a"}, false},
		{"dry cluster", []string{"-kn", "a"}, "/r", nil, []string{"a"}, true},
		{"assignment", []string{"OUT=x", "a"}, "/r", nil, []string{"a"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, ok := readMakeArgs(words(tt.args...), "/r")
			if !ok {
				t.Fatal("readMakeArgs: not ok")
			}
			if a.dir != filepath.Clean(tt.dir) || strings.Join(a.files, " ") != strings.Join(tt.files, " ") ||
				strings.Join(a.targets, " ") != strings.Join(tt.targets, " ") || a.dry != tt.dry {
				t.Fatalf("readMakeArgs(%q) = dir %q files %q targets %q dry %v", tt.args, a.dir, a.files,
					a.targets, a.dry)
			}
		})
	}
}
