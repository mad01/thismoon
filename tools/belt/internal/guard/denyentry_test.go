package guard

import (
	"fmt"
	"strings"
	"testing"
)

// TestDenyEntryRuns pins the entry reader, one block per entry shape: the
// entry, a command as its whitespace-split words, and whether the command
// runs what the entry names.
func TestDenyEntryRuns(t *testing.T) {
	tests := []struct {
		entry, command string
		want           bool
	}{
		// `:*` and a trailing ` *` are the open end.
		{"kubectl delete:*", "kubectl delete pod x", true},
		{"kubectl delete:*", "kubectl get pods", false},
		{"git config --unset *", "git config --unset user.name", true},
		{"git config --unset *", "git config --get user.name", false},

		// An exact entry is a prefix too.
		{"gsutil rm", "gsutil rm gs://b/x", true},
		{"gsutil rm", "gsutil ls gs://b", false},

		// Head verbs are read past global flags, two ways each, for any tool.
		{"git remote prune", "git -C repo remote prune origin", true},
		{"git remote prune", "git -c k=v --no-pager remote prune origin", true},
		{"gh repo delete", "gh -R o/r repo delete --yes", true},
		{"redis-cli FLUSHALL", "redis-cli -h h -n 2 flushall", true},
		{"redis-cli FLUSHALL", "redis-cli -h h get k", false},
		{"git remote prune", "git log remote prune", false},

		// Verbs are whole words.
		{"rails db:drop", "rails db:drop", true},
		{"rails db:drop", "rails db:drop:all", false},
		{"rake db:drop:all", "rake db:drop:all", true},

		// The tool is the command name: a path, a backslash and quotes drop.
		{"psql * DROP *", "/usr/bin/psql -c 'DROP TABLE t'", true},
		{"git config * --unset *", `\git config --unset x`, true},

		// A flag in the head is a required option.
		{"gsutil -m rm", "gsutil -m rm gs://b/x", true},
		{"gsutil -m rm", "gsutil -o Credentials:x -m rm gs://b/x", true},
		{"gsutil -m rm", "gsutil rm gs://b/x", false},
		{"gsutil -m rm", "gsutil -m cp a gs://b/a", false},

		// A required option without a value, anywhere after the tool word.
		{"git config * --unset *", "git config --global --unset user.name", true},
		{"git config * --unset *", "git -C repo config --global --unset x", true},
		{"git config * --unset *", "git config --global user.name x", false},
		{"git config * --unset *", "git config --global --unset-all x", false},

		// A required option with a value.
		{"gh api * -X DELETE *", "gh api -X DELETE /repos/o/r", true},
		{"gh api * -X DELETE *", "gh api -X GET /repos/o/r", false},
		{"gh api * -X DELETE *", "gh api /repos/o/r", false},
		{"gh api * --method DELETE *", "gh api /repos/o/r --method DELETE", true},
		{"gh api * --method DELETE *", "gh api --method GET /repos/o/r", false},

		// Value forms: joined, after `=`, quoted, and case aside.
		{"gh api * -X DELETE *", "gh api -XDELETE /repos/o/r", true},
		{"gh api * -X DELETE *", "gh api -X=DELETE /repos/o/r", true},
		{"gh api * -X DELETE *", "gh api -X delete /repos/o/r", true},
		{"gh api * --method DELETE *", "gh api --method=DELETE /repos/o/r", true},
		{"gh api * --method DELETE *", `gh api --method="DELETE" /repos/o/r`, true},
		{"gh api * --method DELETE *", "gh api --method= DELETE /repos/o/r", true},
		{"gh api * --method DELETE *", "gh api --method=GET /repos/o/r", false},
		{"kubectl scale * --replicas=0 *", "kubectl scale deploy x --replicas=0", true},
		{"kubectl scale * --replicas=0 *", "kubectl scale deploy x --replicas 0", true},
		{"kubectl scale * --replicas=0 *", "kubectl scale deploy x --replicas=3", false},

		// A long option abbreviated to three letters or more.
		{"aws s3 rm * --recursive *", "aws s3 rm s3://b --rec", true},
		{"aws s3 rm * --recursive *", "aws --profile p s3 rm s3://b --recursive", true},
		{"aws s3 rm * --recursive *", "aws s3 rm s3://b --re", false},
		{"aws s3 rm * --recursive *", "aws s3 rm s3://b/k", false},
		{"gh api * --method DELETE *", "gh api --meth DELETE /repos/o/r", true},

		// A one-letter option inside a bundle, and never inside a long one.
		{"docker compose down * -v *", "docker compose down -v", true},
		{"docker compose down * -v *", "docker compose down -qv", true},
		{"docker compose down * -v *", "docker compose down --verbose", false},
		{"docker compose down * -v *", "docker compose down", false},
		{"gh api * -X DELETE *", "gh api -iXDELETE /repos/o/r", true},
		// Past the first letter, a bundle with a non-letter in it is a value.
		{"gh api * -X DELETE *", "gh api -HX-Req:$ID /repos/o/r", false},
		{"docker compose down * -v *", "docker compose down -t5v", false},

		// A value the shell expands may be any value; a word alone is no option.
		{"gh api * -X DELETE *", "gh api -X $METHOD /repos/o/r", true},
		{"gh api * -X DELETE *", `gh api -X "$M" /repos/o/r`, true},
		{"kubectl scale * --replicas=0 *", "kubectl scale deploy x --replicas=$N", true},
		{"gh api * -X DELETE *", "gh api $FLAGS /repos/o/r", false},

		// A phrase: whole words, case aside, after the tool word, quotes collapsed.
		{"psql * DROP *", "psql -h h -d db -c 'drop table t'", true},
		{"psql * DROP *", `psql -c "DROP TABLE t"`, true},
		{"psql * DROP *", "psql -c 'select 1'", false},
		{"psql * DROP *", "psql -d dropzone -c 'select 1'", false},
		{"psql * DELETE FROM *", `psql -c "delete  from t where id = 1"`, true},
		{"psql * DELETE FROM *", "psql -c 'select deleted from t'", false},
		{"sqlite3 * DROP *", "sqlite3 app.db 'DROP TABLE t;'", true},

		// A value glued to its option or to a bundle, and SQL block comments
		// between a phrase's words. A glued value counts when it holds more
		// than one word; a word cut at whitespace inside a quote or after a
		// backslash runs on into the words after it.
		{"psql * DROP *", "psql '-cDROP TABLE t'", true},
		{"psql * DROP *", `psql -cDROP\ TABLE\ t`, true},
		{"psql * TRUNCATE *", `psql "-tActruncate t"`, true},
		{"psql * DROP *", "psql -cDROP TABLE t", false},
		{"psql * DROP *", "psql -dbackdrop -c 'select 1'", false},
		{"psql * DROP *", "psql -cselect 1", false},
		{"psql * DROP *", "psql -ddroptest -cselect 1", false},
		{"psql * DELETE FROM *", "psql -c DELETE/**/FROM t", true},
		{"psql * DELETE FROM *", "psql -c DELETE/*x*/FROM/**/t", true},
		{"psql * DELETE FROM *", "psql -c DELETE/*FROM t", false},

		// A NAME=value word may come before the verbs, and a rake or rails task
		// at any position.
		{"rake db:drop", "rake RAILS_ENV=production db:drop", true},
		{"rails db:reset", "rails RAILS_ENV=x db:reset", true},
		{"rake db:drop", "rake db:create db:drop", true},
		{"rake db:drop", "rake db:create db:migrate", false},
		{"kubectl delete", "kubectl get pods delete", false},

		// A word in ANSI-C quoting reads as the shell passes it.
		{"kubectl delete", "kubectl $'delete' pod x", true},
		{"kubectl delete", `kubectl $'\x64elete' pod x`, true},
		{"helm uninstall", "$'helm' uninstall r", true},
		{"kubectl delete", "kubectl $'get' pods", false},

		// Options end at `--`; verbs read past it.
		{"git config * --unset *", "git config -- --unset", false},
		{"gh api * -X DELETE *", "gh api -- -X DELETE", false},
		{"kubectl delete", "kubectl -n x -- delete pod", true},

		// The tool rules: gcloud release tracks, apply -destroy, rm's reader.
		{"gcloud secrets delete", "gcloud --project p beta secrets delete s", true},
		{"gcloud secrets delete", "gcloud beta secrets list", false},
		{"tofu destroy", "tofu -chdir=infra apply -destroy", true},
		{"tofu destroy", "tofu plan -destroy", false},
		{"terraform destroy", "terraform apply -destroy", true},
		{"rm -rf", "rm -r -f build", true},
		{"rm -rf *", "rm --recursive --force build", true},
		{"rm -rf", "rm -f build", false},
	}
	for _, tt := range tests {
		t.Run(tt.entry+"/"+tt.command, func(t *testing.T) {
			e, ok := readDenyEntry(tt.entry)
			if !ok {
				t.Fatalf("readDenyEntry(%q) refused the entry", tt.entry)
			}
			if got := e.runs(strings.Fields(tt.command)); got != tt.want {
				t.Fatalf("runs(%q) = %v, want %v (entry %+v)", tt.command, got, tt.want, e)
			}
		})
	}
}

// TestDenyEntryPhraseComments pins the SQL comments a phrase reads past, on
// a -c value that holds newlines: a block comment anywhere, and a line
// comment up to the newline that ends it.
func TestDenyEntryPhraseComments(t *testing.T) {
	e, ok := readDenyEntry("psql * DELETE FROM *")
	if !ok {
		t.Fatal("readDenyEntry refused the entry")
	}
	tests := []struct {
		sql  string
		want bool
	}{
		{"DELETE -- x\nFROM t", true},
		{"DELETE --\nFROM t", true},
		{"DELETE # x\nFROM t", true},
		{"DELETE /* x\n */ FROM t", true},
		{"DELETE -- a\n-- b\n\tFROM t", true},
		{"DELETE --x FROM t", false},
		{"DELETE # x FROM t", false},
		{"select 1 -- DELETE x\nFROM t", false},
	}
	for _, tt := range tests {
		if got := e.runs([]string{"psql", "-c", tt.sql}); got != tt.want {
			t.Errorf("runs(psql -c %q) = %v, want %v", tt.sql, got, tt.want)
		}
	}
}

// TestReadDenyEntryRefuses pins the shapes the reader leaves to the text
// match.
func TestReadDenyEntryRefuses(t *testing.T) {
	for _, entry := range []string{
		"",
		"*",
		"rm -rf /tmp/*",
		"git push origin *main*",
		"git config -- x",
		"gh api * -X DELETE extra *",
		"rm --no-such-option",
	} {
		if e, ok := readDenyEntry(entry); ok {
			t.Errorf("readDenyEntry(%q) = %+v, want refused", entry, e)
		}
	}
}

// TestScriptDenyListReadsWildcardEntries runs settings entries with `*`
// groups and flags in them through the carriers the guard reads, with a
// read-only sibling of each that must pass.
func TestScriptDenyListReadsWildcardEntries(t *testing.T) {
	dir := t.TempDir()
	g := NewScriptDenyList(scriptDenyConfig([]string{
		"git config * --unset *", "gh api * -X DELETE *", "psql * DROP *", "gsutil -m rm",
	}, nil))
	forms := []struct {
		command string
		deny    bool
	}{
		{"git -C repo config --global --unset user.name", true},
		{"git -C repo config --global --get user.name", false},
		{"gh -R o/r api -XDELETE /repos/o/r", true},
		{"gh -R o/r api -X GET /repos/o/r", false},
		{`psql -h h -U u -d db -c "drop table t"`, true},
		{`psql -h h -U u -d db -c "select 1"`, false},
		{"gsutil -o x:y -m rm gs://b/x", true},
		{"gsutil -m ls gs://b", false},
	}
	for i, form := range forms {
		script := writeScript(t, dir, fmt.Sprintf("wild%d.sh", i), form.command+"\n")
		carriers := map[string]string{
			"bash -c":     "bash -c '" + strings.ReplaceAll(form.command, "'", `'\''`) + "'",
			"script file": "bash " + script,
			"sudo":        "sudo " + form.command,
		}
		for carrier, command := range carriers {
			t.Run(carrier+"/"+form.command, func(t *testing.T) {
				d := g.Check(Input{Event: EventBash, Command: command})
				if got := d != nil; got != form.deny {
					t.Fatalf("%s: deny = %v, want %v (denial: %+v)", command, got, form.deny, d)
				}
			})
		}
	}
}
