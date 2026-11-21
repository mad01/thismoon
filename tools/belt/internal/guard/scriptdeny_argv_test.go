package guard

import (
	"fmt"
	"strings"
	"testing"
)

// argvDeny is a deny list shaped like the live one: settings entries for the
// verb tools and the extra rm patterns.
var argvDeny = []string{
	"kubectl delete",
	"gcloud sql instances delete",
	"gcloud compute instances delete",
	"terraform destroy",
}

var argvExtra = []string{"rm -rf", "rm -fr"}

// argvForms are commands that run a deny-listed operation in a spelling the
// text match misses, or a near spelling that must keep passing. None holds a
// quote, so each fits every carrier below.
var argvForms = []struct {
	name    string
	command string
	deny    bool
}{
	{"rm split flags", "rm -r -f build", true},
	{"rm split capital R", "rm -R -f build", true},
	{"rm fr bundle", "rm -fr build", true},
	{"rm bundle with verbose", "rm -rfv build", true},
	{"rm capital bundle with verbose", "rm -Rfv build", true},
	{"rm bundle then end of options", "rm -rf -- build", true},
	{"rm long options", "rm --recursive --force build", true},
	{"rm long option prefixes", "rm --rec --for build", true},
	{"rm options after the operand", "rm build -r -f", true},
	{"rm by path", "/bin/rm -r -f build", true},
	{"rm under sudo", "sudo rm -r -f build", true},
	{"rm through xargs", "find build -print0 | xargs -0 rm -r -f", true},
	{"rm brace bundle", "rm -{r,f} build", true},
	{"rm force without recursion", "rm -f build/out.txt", false},
	{"rm recursion without force", "rm -r /tmp/scratch", false},
	{"rm dir is not recursive", "rm -df build", false},
	{"rm flags after end of options", "rm -- -rf", false},
	{"rm ambiguous long prefix", "rm -r --v build", false},
	{"rm expanded word is not an option", "rm -r $FLAGS build", false},

	{"kubectl short namespace", "kubectl -n prod delete pod x", true},
	{"kubectl long namespace", "kubectl --namespace=prod delete pod x", true},
	{"kubectl context", "kubectl --context x delete pod y", true},
	{"kubectl kubeconfig", "kubectl --kubeconfig cfg delete pod y", true},
	{"kubectl unknown global flag", "kubectl --not-a-flag v delete pod y", true},
	{"kubectl expanded flags", "kubectl $FLAGS delete pod y", true},
	{"kubectl get with namespace", "kubectl get pods -n x", false},
	{"kubectl namespace then get", "kubectl -n prod get pods", false},

	{"gcloud project", "gcloud --project p sql instances delete db", true},
	{"gcloud project equals", "gcloud --project=p compute instances delete vm", true},
	{"gcloud quiet", "gcloud -q sql instances delete db", true},
	{"gcloud format and quiet", "gcloud --format json --quiet sql instances delete db", true},
	{"gcloud flag between groups", "gcloud compute --project=p instances delete vm", true},
	{"gcloud beta track", "gcloud beta sql instances delete db", true},
	{"gcloud alpha track after a flag", "gcloud --project p alpha compute instances delete vm", true},
	{"gcloud beta track list", "gcloud beta sql instances list", false},
	{"gcloud project list", "gcloud --project p compute instances list", false},
	{"gcloud project describe", "gcloud --project p sql instances describe db", false},

	{"terraform chdir", "terraform -chdir=infra destroy", true},
	{"terraform chdir auto approve", "terraform -chdir=infra destroy -auto-approve", true},
	{"terraform apply destroy", "terraform apply -destroy -auto-approve", true},
	{"terraform chdir apply destroy", "terraform -chdir=infra apply -auto-approve -destroy", true},
	{"terraform apply double dash destroy", "terraform apply --destroy", true},
	{"terraform apply destroy true", "terraform apply -destroy=true", true},
	{"terraform plan destroy", "terraform plan -destroy", false},
	{"terraform chdir plan destroy", "terraform -chdir=infra plan -destroy -out=plan", false},
	{"terraform apply destroy false", "terraform apply -destroy=false", false},
}

// TestScriptDenyListArgv runs every argv form through each carrier the guard
// reads: a -c string, eval, a print piped into sh, and a script file.
func TestScriptDenyListArgv(t *testing.T) {
	dir := t.TempDir()
	g := NewScriptDenyList(scriptDenyConfig(argvDeny, argvExtra))
	for i, form := range argvForms {
		script := writeScript(t, dir, fmt.Sprintf("form%d.sh", i), form.command+"\n")
		carriers := map[string]string{
			"bash -c":      fmt.Sprintf("bash -c '%s'", form.command),
			"eval":         fmt.Sprintf("eval \"%s\"", form.command),
			"pipe into sh": fmt.Sprintf("echo '%s' | sh", form.command),
			"script file":  "bash " + script,
		}
		for carrier, command := range carriers {
			t.Run(carrier+"/"+form.name, func(t *testing.T) {
				d := g.Check(Input{Event: EventBash, Command: command})
				if got := d != nil; got != form.deny {
					t.Fatalf("%s: deny = %v, want %v (denial: %+v)", command, got, form.deny, d)
				}
			})
		}
	}
}

// TestScriptDenyListArgvIndirection covers the indirection heads the guard
// scans at the top level.
func TestScriptDenyListArgvIndirection(t *testing.T) {
	g := NewScriptDenyList(scriptDenyConfig(argvDeny, argvExtra))
	tests := []struct {
		command string
		deny    bool
	}{
		{"find build -print0 | xargs -0 rm -r -f", true},
		{"find build -exec rm -r -f {} +", true},
		{"kubectl get pods -o name | xargs kubectl -n prod delete", true},
		{"find build -print0 | xargs -0 rm -f", false},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			d := g.Check(Input{Event: EventBash, Command: tt.command})
			if got := d != nil; got != tt.deny {
				t.Fatalf("deny = %v, want %v (denial: %+v)", got, tt.deny, d)
			}
		})
	}
}

// TestDenyRunsWords pins the argv match as a function over one command's
// words, the command name first.
func TestDenyRunsWords(t *testing.T) {
	tests := []struct {
		pattern string
		words   string
		want    bool
	}{
		{"rm -rf", "rm -r -f x", true},
		{"rm -rf", "RM -r -f x", true},
		{`rm -rf`, `\rm -fr x`, true},
		{"rm -rf", "rm --force --recursive x", true},
		{"rm -rf", "rm -r x", false},
		{"rm -rf", "rm -f --dir x", false},
		{"rm -rf", "sudo rm -r -f x", false},
		{"rm -rf /", "rm -r -f /", true},
		{"rm -rf /", "rm -r -f /tmp", false},
		{"rm", "rm x", true},
		{"rm --no-such-option", "rm --no-such-option x", false},
		{"kubectl delete", "kubectl -n x delete pod", true},
		{"kubectl delete", "kubectl -n x -- delete pod", true},
		{"kubectl delete", "kubectl get -n delete", false},
		{"kubectl delete --all", "kubectl delete --all", false},
		{"gcloud sql instances delete", "gcloud sql --quiet instances delete db", true},
		{"gcloud sql instances delete", "gcloud sql backups delete db", false},
		{"terraform destroy", "terraform apply -destroy=false -destroy", true},
		{"terraform destroy", "terraform apply -destroy=maybe", true},
		{"terraform destroy", "terraform apply -destroy -destroy=false", false},
		{"bq rm", "bq --project_id=p rm x", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"/"+tt.words, func(t *testing.T) {
			d := compilePatterns([]string{tt.pattern})[0]
			if got := d.runsWords(strings.Fields(tt.words)); got != tt.want {
				t.Fatalf("runsWords(%q) = %v, want %v", tt.words, got, tt.want)
			}
		})
	}
}
