package guard

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// trancheEntries are the first-tranche Bash deny entries as the Claude
// settings give them (cfg.ClaudeDeny: `Bash(kubectl delete:*)` reads as
// `kubectl delete`), plus the kubectl, gcloud and terraform entries the
// settings carried before. A verb's aliases are entries of their own
// (`pulumi down`, `gh secret remove`), since the reader matches the verb as
// written.
var trancheEntries = []string{
	"psql * DROP *", "psql * TRUNCATE *", "psql * DELETE FROM *",
	"mysql * DROP *", "mysql * TRUNCATE *", "mysql * DELETE FROM *",
	"mariadb * DROP *", "mariadb * TRUNCATE *", "mariadb * DELETE FROM *",
	"sqlite3 * DROP *", "sqlite3 * DELETE FROM *",
	"dropdb", "mysqladmin drop",
	"redis-cli FLUSHALL", "redis-cli FLUSHDB",
	"prisma migrate reset", "prisma db push * --accept-data-loss *", "prisma db push * --force-reset *",
	"rails db:drop", "rails db:drop:all", "rails db:reset", "rails db:purge",
	"rake db:drop", "rake db:drop:all", "rake db:reset", "rake db:purge",
	"terraform destroy", "terraform state rm", "terraform workspace delete",
	"tofu destroy", "tofu state rm", "tofu workspace delete",
	"pulumi destroy", "pulumi down", "pulumi dn", "pulumi stack rm", "pulumi stack remove", "pulumi stack delete",
	"pulumi state delete", "pulumi state remove", "pulumi state rm",
	"aws s3 rm * --recursive *", "aws s3 rb * --force *", "aws ec2 terminate-instances",
	"aws rds delete-db-instance", "aws rds delete-db-cluster", "aws dynamodb delete-table",
	"aws cloudformation delete-stack",
	"az group delete",
	"gcloud secrets delete", "gcloud kms keys versions destroy", "gcloud storage buckets delete",
	"gcloud sql instances delete",
	"gh repo delete", "gh repo archive", "gh release delete", "gh issue delete", "gh secret delete",
	"gh secret remove", "gh variable delete", "gh variable remove", "gh api * -X DELETE *", "gh api * --method DELETE *",
	"gh repo edit * --visibility public *", "gh pr merge * --admin *",
	"docker system prune * --volumes *", "docker volume rm", "docker volume remove", "docker volume prune",
	"docker compose down * -v *", "docker compose down * --volumes *",
	"docker-compose down * -v *", "docker-compose down * --volumes *",
	"podman system reset", "podman volume rm", "podman volume remove", "podman volume prune",
	"kind delete cluster", "kind delete clusters", "minikube delete",
	"k3d cluster delete", "k3d cluster del", "k3d cluster rm",
	"helm uninstall", "helm delete", "helm del", "helm un",
	"kubectl delete", "kubectl drain", "kubectl scale * --replicas=0 *",
}

// directBlock is one tranche CLI's rows. The plain deny form is the tool,
// then plain. Each word in before goes between the two: a value-taking
// global flag with its value, an --opt=v, a bundle, a $VAR value. The
// wrappers (sudo, env, timeout), a full path and a backslash are added to
// the plain form for every block. denies and passes are written out after
// the tool: more deny forms, and the read-only siblings.
type directBlock struct {
	tool, plain string
	before      []string
	denies      []string
	passes      []string
}

var directBlocks = []directBlock{
	{
		tool: "kubectl", plain: "delete pod x",
		before: []string{
			"-n prod", "--context c", "--kubeconfig k.yaml", "-s https://h:6443",
			"--namespace=prod", "-nprod", `-n "$NS"`, "-v6 -n prod",
		},
		denies: []string{
			"drain node-1", "scale deploy x --replicas=0", "scale deploy x --replicas 0",
			"-n prod scale --replicas=0 deploy/x", "$'delete' pod x", `$'\x64elete' pod x`,
		},
		passes: []string{
			"get pods", "-n prod get pods", "scale deploy x --replicas=3", "describe pod x",
			"logs pod-x", "-n delete-me get pods -l app=x", "$'get' pods",
		},
	},
	{
		tool: "gcloud", plain: "secrets delete s",
		before: []string{
			"--project p", "--account a@x.dev", "--configuration c", "--impersonate-service-account sa@p.dev",
			"beta", "--project=p", "-q", `--project "$P"`,
		},
		denies: []string{
			"kms keys versions destroy 1 --key k --keyring r --location l",
			"storage buckets delete gs://b", "--project p alpha storage buckets delete gs://b",
			"--project p beta sql instances delete db",
		},
		passes: []string{
			"secrets list", "secrets versions access latest --secret s", "storage buckets list",
			"kms keys list --keyring r", "sql instances describe db",
		},
	},
	{
		tool: "aws", plain: "ec2 terminate-instances --instance-ids i-1",
		before: []string{
			"--profile p", "--region eu-west-1", "--endpoint-url http://localhost:4566", "--output json",
			"--profile=p", "--debug --region eu-west-1", `--region "$AWS_REGION"`,
		},
		denies: []string{
			"s3 rm s3://b --recursive", "s3 rm s3://b/k --rec", "--profile p s3 rm s3://b/p/ --recursive",
			"s3 rb s3://b --force", "rds delete-db-instance --db-instance-identifier d",
			"rds delete-db-cluster --db-cluster-identifier c", "dynamodb delete-table --table-name t",
			"cloudformation delete-stack --stack-name s",
		},
		passes: []string{
			"s3 rm s3://b/k", "s3 rb s3://b", "s3 ls s3://b --recursive", "ec2 describe-instances",
			"rds describe-db-instances", "dynamodb describe-table --table-name t",
		},
	},
	{
		tool: "az", plain: "group delete -n g --yes",
		before: []string{"--subscription s", "--subscription=s", "--only-show-errors", `--subscription "$SUB"`},
		passes: []string{"group list", "group show -n g", "group create -n g -l westeurope"},
	},
	{
		tool: "gh", plain: "repo delete o/r --yes",
		before: []string{"-R o/r", "--repo o/r", "--repo=o/r", `-R "$REPO"`},
		denies: []string{
			"repo archive o/r", "release delete v1", "issue delete 1", "secret delete S",
			"variable delete V", "api -X DELETE repos/o/r", "api repos/o/r -XDELETE",
			"api --method DELETE repos/o/r", "api --method=delete repos/o/r", `api -X "$M" repos/o/r`,
			"api -X=DELETE repos/o/r", "repo edit o/r --visibility public",
			"repo edit --visibility=public --accept-visibility-change-consequences",
			"pr merge 1 --admin --squash", "-R o/r pr merge 1 --admin", "secret remove S",
			"variable remove V",
		},
		passes: []string{
			"api -X GET repos/o/r", "api repos/o/r", "api --method POST repos/o/r/issues",
			"repo edit --visibility private", "pr merge 1 --squash", "repo view o/r", "release list",
			"issue list", "secret list",
		},
	},
	{
		tool: "docker", plain: "volume rm v1",
		before: []string{
			"-H unix:///x.sock", "--context c", "-c c", "--config /x", "--context=c", "-Dl debug",
			`-H "$DOCKER_HOST"`,
		},
		denies: []string{
			"volume prune -f", "system prune --volumes", "system prune -af --volumes",
			"compose down -v", "compose down --volumes", "compose -f x.yml down -v",
			"compose -p proj down --volumes --remove-orphans", "volume remove v1",
		},
		passes: []string{
			"system prune -f", "compose down", "compose down --remove-orphans", "volume ls", "ps",
			"compose up -d", "volume inspect v1",
		},
	},
	{
		tool: "docker-compose", plain: "down -v",
		before: []string{"-f x.yml", "-p proj", "--file=x.yml", `-f "$F"`},
		denies: []string{"down --volumes", "down --rmi all -v"},
		passes: []string{"down", "up -d", "ps", "logs -f"},
	},
	{
		tool: "podman", plain: "volume rm v1",
		before: []string{"--connection c", "--url unix:///x.sock", "--connection=c", `--url "$U"`},
		denies: []string{"system reset -f", "volume prune -f", "volume remove v1"},
		passes: []string{"volume ls", "system df", "ps -a"},
	},
	{
		tool: "psql", plain: "-c 'DROP TABLE t'",
		before: []string{"-h h", "-p 5432", "-U u", "-d db", "--host=h", "-qX", `-d "$DB"`},
		denies: []string{
			"-c 'truncate t'", `db -c "DELETE FROM t WHERE id = 1"`, "-c 'drop database x'",
			"postgres://u@h/db -c 'DROP SCHEMA s CASCADE'", "-c'DROP TABLE t'", `-tAc"truncate t"`,
			`-c "DELETE/**/FROM t"`, `-c "DELETE /* x */ FROM t"`,
		},
		passes: []string{
			"-c 'select 1'", "-l", "-c 'select * from dropped_items'", "-d droptest -c 'select 1'",
			"-c 'select deleted from t'", "-c'select 1'", `-tAc"select count(*) from t"`,
			"-ddroptest -c'select 1'",
		},
	},
	{
		tool: "mysql", plain: "-e 'DROP TABLE t'",
		before: []string{"-h h", "-P 3306", "-u u", "-D db", "--host=h", "-uroot", `-p"$PW"`},
		denies: []string{
			"db -e 'TRUNCATE t'", "-e 'delete from t'", `-e"DROP DATABASE x"`,
			`-uroot -p"$PW" -e"DROP DATABASE x"`, `db -e"delete from t"`, `-e"DELETE/**/FROM t"`,
		},
		passes: []string{
			"-e 'select 1'", "-e 'show tables'", "db < /dev/null", `-e"show tables"`,
			`-uroot -p"$PW" -e"select 1"`,
		},
	},
	{
		tool: "mariadb", plain: "-e 'DROP DATABASE x'",
		before: []string{"-h h", "-u u", "--host=h", "-uroot", `-D "$DB"`},
		denies: []string{"-e 'TRUNCATE t'", "-e 'DELETE FROM t'", `-e"DROP DATABASE x"`, "-uroot -e'truncate t'"},
		passes: []string{"-e 'select 1'", `-e"show tables"`},
	},
	{
		tool: "sqlite3", plain: "app.db 'DROP TABLE t'",
		before: []string{"-bail", "-cmd '.timeout 1000'", "-header -csv", `-init "$RC"`},
		denies: []string{"app.db 'DELETE FROM t'", `-cmd"DROP TABLE t" app.db`, `app.db "DELETE/**/FROM t"`},
		passes: []string{"app.db 'select 1'", "app.db .tables", "app.db '.schema t'", `-cmd".timeout 1000" app.db .tables`},
	},
	{
		tool: "dropdb", plain: "mydb",
		before: []string{"-h h", "-U u", "-p 5432", "--host=h", "-ef", `-U "$PGUSER"`},
	},
	{
		tool: "mysqladmin", plain: "drop db",
		before: []string{"-u root", "-h h", "-uroot", "--force", `-p"$PW"`},
		passes: []string{"status", "ping", "-u root create db"},
	},
	{
		tool: "redis-cli", plain: "FLUSHALL",
		before: []string{"-h h", "-p 6379", "-n 2", "-a pw", "-n2", `-a "$PW"`, "--no-auth-warning"},
		denies: []string{"flushdb", "-h h flushall async"},
		passes: []string{"get k", "-n 2 keys '*'", "info", "-h h ping"},
	},
	{
		tool: "prisma", plain: "migrate reset --force",
		before: []string{"--schema s.prisma", "--schema=s.prisma", `--schema "$S"`},
		denies: []string{"db push --accept-data-loss", "db push --force-reset", "db push --schema s.prisma --accept-data-loss"},
		passes: []string{"migrate status", "db push", "migrate dev", "generate"},
	},
	{
		tool: "rails", plain: "db:drop",
		before: []string{"-e production", "--trace"},
		denies: []string{
			"db:drop:all", "db:reset", "db:purge", "db:drop DISABLE_DATABASE_ENVIRONMENT_CHECK=1",
			"RAILS_ENV=x db:reset", "db:create db:purge",
		},
		passes: []string{"db:migrate", "db:migrate:status", "db:seed", "db:dropper", "RAILS_ENV=x db:migrate", "db:create db:migrate"},
	},
	{
		tool: "rake", plain: "db:drop",
		before: []string{"-f Rakefile", "--trace", "-f=Rakefile"},
		denies: []string{"db:drop:all", "db:reset", "db:purge", "RAILS_ENV=production db:drop", "db:create db:drop"},
		passes: []string{"db:migrate", "-T", "db:create db:migrate"},
	},
	{
		tool: "terraform", plain: "state rm aws_x.y",
		before: []string{"-chdir=infra", `-chdir="$DIR"`},
		denies: []string{"destroy", "workspace delete dev", "apply -destroy", "-chdir=infra apply -auto-approve -destroy"},
		passes: []string{"plan", "state list", "workspace list", "plan -destroy", "apply -destroy=false"},
	},
	{
		tool: "tofu", plain: "destroy",
		before: []string{"-chdir=infra", `-chdir="$DIR"`},
		denies: []string{"apply -destroy", "apply -destroy=true", "state rm a.b", "workspace delete dev"},
		passes: []string{"plan", "apply -destroy=false", "plan -destroy", "state list"},
	},
	{
		tool: "pulumi", plain: "destroy --yes",
		before: []string{"--cwd x", "-C x", "-s dev", "--cwd=x", `-s "$STACK"`},
		denies: []string{
			"stack rm dev", "state delete urn:x", "down --yes", "dn", "stack remove dev", "stack delete dev",
			"state remove urn:x", "state rm urn:x",
		},
		passes: []string{"preview", "stack ls", "up", "state list"},
	},
	{
		tool: "kind", plain: "delete cluster --name x",
		before: []string{"-v 1", "--verbosity=1", "-q"},
		denies: []string{"delete clusters --all"},
		passes: []string{"get clusters", "create cluster", "export kubeconfig"},
	},
	{
		tool: "minikube", plain: "delete",
		before: []string{"-p prof", "--profile=prof", `-p "$P"`},
		denies: []string{"delete --all --purge"},
		passes: []string{"status", "start", "profile list"},
	},
	{
		tool: "k3d", plain: "cluster delete x",
		before: []string{"--verbose", "--timestamps"},
		denies: []string{"cluster rm x", "cluster del x"},
		passes: []string{"cluster list", "cluster create x"},
	},
	{
		tool: "helm", plain: "uninstall rel",
		before: []string{"-n ns", "--kube-context c", "--kubeconfig k", "--namespace=ns", "-nns", `-n "$NS"`},
		denies: []string{"delete rel", "del rel", "un rel", "-n ns uninstall rel --keep-history", "$'uninstall' r"},
		passes: []string{"list", "status rel", "upgrade rel chart", "history rel", "undo rel"},
	},
}

// denyCommands returns the block's deny rows: the plain form, each before
// word in front of it, the plain form behind sudo, env and timeout, by a
// full path and behind a backslash, then the written-out denies.
func (b directBlock) denyCommands() []string {
	plain := b.tool + " " + b.plain
	out := []string{plain}
	for _, w := range b.before {
		out = append(out, b.tool+" "+w+" "+b.plain)
	}
	out = append(out,
		"sudo "+plain,
		"sudo -u root -E "+plain,
		"env A=1 "+plain,
		"timeout 30 "+plain,
		"A=1 "+plain,
		"/usr/local/bin/"+plain,
		`\`+plain,
	)
	for _, d := range b.denies {
		out = append(out, b.tool+" "+d)
	}
	return out
}

// passCommands returns the block's pass rows: the read-only siblings, and
// an echo and a commit message that name the plain deny form.
func (b directBlock) passCommands() []string {
	var out []string
	for _, p := range b.passes {
		out = append(out, b.tool+" "+p)
	}
	plain := b.tool + " " + b.plain
	return append(out,
		"echo "+shellQuote(plain),
		"git commit -m "+shellQuote("docs: deny "+plain),
	)
}

// directShapes are rows on the shell around a command, the same for every
// CLI: a command after & or in ( ) runs too, and a comment, a heredoc body,
// a quoted string and a -c string do not run at the top level (the last is
// script-deny-list's).
var directShapes = []struct {
	command string
	deny    bool
}{
	{"sleep 1 & kubectl delete pod x", true},
	{"(kubectl delete pod x)", true},
	{"( cd x && kubectl delete pod x )", true},
	{"if true; then kubectl delete pod x; fi", true},
	{"for p in a b; do kubectl delete pod $p; done", true},
	{"! kubectl delete pod x", true},
	{"{ kubectl delete pod x; }", true},
	{"watch -n 5 kubectl delete pod x", true},
	{"env -S 'kubectl delete pod x'", true},
	{"nohup kubectl delete pod x &", true},
	{"time kubectl delete pod x", true},
	{"command kubectl delete pod x", true},
	{`"kubectl" "delete" pod x`, true},
	{"k\\ubectl delete pod x", true},
	{"cd /tmp && kubectl delete pod x 2>&1 | tee out.log", true},
	{"# kubectl delete pod x", false},
	{"kubectl get pods # then kubectl delete pod x", false},
	{"rg -n 'helm uninstall|kubectl delete' docs/", false},
	{"grep -c 'x; kubectl delete pod' notes.md", false},
	{"cat > notes.md <<'EOF'\nkubectl delete pod x\nEOF", false},
	{"bash -c 'kubectl delete pod x'", false},
	{"echo $(date) kubectl delete", false},
	{"kubectl get pods -o name | xargs echo", false},
	{"git commit -m 'block kubectl delete' -m 'and terraform destroy'", false},
	{"RAILS_ENV=x rake db:migrate", false},
	// Lines before one the reader cannot follow run first, and the line it
	// cannot follow is read with its quotes and backslashes dropped.
	{"kube\\ctl delete pod x\necho 'unterminated", true},
	{"gh api -X D\"ELETE\" repos/o/r\necho 'unterminated", true},
	{"gh api -X D\"ELETE\" repos/o/r 'unterminated", true},
	{"kubectl get pods\necho 'unterminated", false},
	{"mysql db -e\"delete from t\" 'unterminated", true},
	{"kubectl $'delete' pod x 'unterminated", true},
	{"rg 'a; kubectl delete' docs\necho 'unterminated", false},
	{"sudo kubectl $'delete' pod x", true},
	// `function f {` is an opener the reader reads past, so the body reads as
	// top-level commands; the body of `f() {` is left to script-deny-list.
	{"function f { kubectl delete pod x; }", true},
	{"f() { kubectl delete pod x; }; f", false},
}

// TestDirectDenyListTranche: each tranche CLI's direct forms are denied,
// read past global flags, quoting and wrappers, and its read-only siblings,
// an echo and a commit message that name an entry pass.
func TestDirectDenyListTranche(t *testing.T) {
	g := NewDirectDenyList(config.Config{ClaudeDeny: trancheEntries})
	for _, b := range directBlocks {
		t.Run(b.tool, func(t *testing.T) {
			for _, c := range b.denyCommands() {
				if d := g.Check(Input{Event: EventBash, Command: c}); d == nil {
					t.Errorf("allows a direct deny-listed form:\n%s", c)
				}
			}
			for _, c := range b.passCommands() {
				if d := g.Check(Input{Event: EventBash, Command: c}); d != nil {
					t.Errorf("denies a pass row:\n%s\n%s", c, d.Reason)
				}
			}
		})
	}
	t.Run("shell shapes", func(t *testing.T) {
		for _, s := range directShapes {
			if got := g.Check(Input{Event: EventBash, Command: s.command}) != nil; got != s.deny {
				t.Errorf("deny = %v, want %v:\n%s", got, s.deny, s.command)
			}
		}
	})
}

// directLaunchers are rows on a launcher in front of a tranche command,
// deny and pass per launcher form. Any word after the launcher may start
// the command, so its own options need no table. These rows sit outside the
// frozen corpus, whose groups only grow at the end.
var directLaunchers = []struct {
	command string
	deny    bool
}{
	{"npx -p prisma prisma migrate reset", true},
	{"npx prisma migrate reset --force", true},
	{"npx prisma migrate status", false},
	{"pnpx prisma migrate reset", true},
	{"pnpx prisma generate", false},
	{"bunx prisma db push --accept-data-loss", true},
	{"bunx prisma db push", false},
	{"bun x prisma migrate reset", true},
	{"bun x prisma migrate status", false},
	{"npm exec -- prisma migrate reset", true},
	{"npm x prisma migrate reset", true},
	{"npm exec prisma migrate status", false},
	{"pnpm prisma migrate reset", true},
	{"pnpm --filter api exec prisma migrate reset", true},
	{"pnpm dlx prisma migrate reset", true},
	{"pnpm prisma migrate status", false},
	{"yarn prisma migrate reset", true},
	{"yarn workspace api prisma db push --force-reset", true},
	{"yarn dlx prisma migrate reset", true},
	{"yarn prisma migrate status", false},
	{"bundle exec rake db:drop", true},
	{"bundle exec rails db:reset", true},
	{"bundle exec rake db:migrate:status", false},
	{"poetry run psql -c 'DROP TABLE t'", true},
	{"poetry run pytest", false},
	{"uv run --with x prisma migrate reset", true},
	{"uv run --with x prisma generate", false},
	{"mise exec -- terraform state rm x", true},
	{"mise x terraform@1.9 -- terraform destroy", true},
	{"mise exec -- terraform plan", false},
	{"direnv exec . tofu destroy", true},
	{"direnv exec . tofu plan", false},
	{"docker exec -it db psql -U postgres -c 'DROP TABLE t'", true},
	{"docker exec db redis-cli FLUSHALL", true},
	{"docker exec -it db psql -c 'select 1'", false},
	{"docker compose exec db psql -c 'DROP TABLE t'", true},
	{"docker compose -f x.yml exec -T db mysql -e 'TRUNCATE t'", true},
	{"docker --context c compose exec db psql -c 'DROP TABLE t'", true},
	{"docker compose exec db psql -c 'select 1'", false},
	{"kubectl exec -it pod -- psql -c 'DROP TABLE t'", true},
	{"kubectl -n prod exec deploy/api -c app -- rake db:drop", true},
	{"kubectl exec pod -- kubectl delete pod x", true},
	{"kubectl exec -it pod -- psql -c 'select 1'", false},
	{"docker container exec db dropdb x", true},
	{"docker container exec db ls", false},
	{"docker-compose exec db dropdb x", true},
	{"docker-compose exec db psql -c 'select 1'", false},
	{"podman exec -it db psql -c 'DROP TABLE t'", true},
	{"podman exec db ls", false},
	{"uvx --from awscli aws s3 rm s3://b --recursive", true},
	{"uvx ruff check .", false},
	{"pipx run --spec awscli aws s3 rm s3://b --recursive", true},
	{"pipx run black --check .", false},
	{"docker run --rm -it postgres psql -h db -c 'DROP TABLE t'", true},
	{"docker run img ls", false},
	{"docker container run --rm redis redis-cli -h cache FLUSHALL", true},
	{"docker container run --rm img ls", false},
	{"docker compose run --rm web rake db:drop", true},
	{"docker compose run --rm web rake db:migrate", false},
	{"docker-compose run web rails db:reset", true},
	{"docker-compose run web rails db:migrate", false},
	{"kubectl run tmp --rm -it --image=postgres -- psql -c 'DROP TABLE t'", true},
	{"kubectl run tmp --image=busybox -- ls", false},
	{"kubectl debug -it pod --image=redis -- redis-cli FLUSHDB", true},
	{"kubectl debug -it pod --image=busybox -- ls", false},
	// A launcher behind a wrapper or another launcher, or named by its path.
	{"sudo docker exec db dropdb app", true},
	{"timeout 30 kubectl exec pod -- redis-cli flushdb", true},
	{"bundle exec npx prisma migrate reset", true},
	{"/usr/local/bin/npx prisma migrate reset", true},
	// A word after a launcher that only happens to start an entry denies
	// too, a false deny the design accepts.
	{"yarn test dropdb", true},
	// Only a launcher's later words are read: a subcommand that runs no
	// command, an echo and a commit message pass.
	{"kubectl logs pod -c dropdb", false},
	{"docker logs dropdb", false},
	{"echo npx prisma migrate reset", false},
	{"git commit -m 'npx prisma migrate reset'", false},
}

// TestDirectDenyListReadsPastLaunchers: a tranche command behind a launcher
// is denied, and the same launcher in front of a read-only sibling passes.
func TestDirectDenyListReadsPastLaunchers(t *testing.T) {
	g := NewDirectDenyList(config.Config{ClaudeDeny: trancheEntries})
	for _, tt := range directLaunchers {
		if got := g.Check(Input{Event: EventBash, Command: tt.command}) != nil; got != tt.deny {
			t.Errorf("deny = %v, want %v:\n%s", got, tt.deny, tt.command)
		}
	}
}

// TestLaunchedDenyReadsEveryWord: the launcher read skips only the words no
// entry may start with (mayStartAt), so it reaches the verdict that trying
// every entry at every later word reaches, and the entry the word-by-word
// read finds, for the tranche rows behind a launcher, split as text and as
// the shell passes them.
func TestLaunchedDenyReadsEveryWord(t *testing.T) {
	patterns := compilePatterns(append(slices.Clone(trancheEntries), "rm -rf", "kubectl delete", `re:\bwipefs\b`))
	commands := []string{
		"'' kubectl delete pod x", `"kubectl" "delete" pod x`, `[ "kubectl", "delete" ]`,
		"' kubectl delete pod x'", `"" rm -rf /x`, "x wipefs -a /dev/sdb",
	}
	for _, b := range directBlocks {
		commands = append(commands, b.denyCommands()...)
		commands = append(commands, b.passCommands()...)
	}
	for _, s := range directShapes {
		commands = append(commands, s.command)
	}
	for _, l := range directLaunchers {
		commands = append(commands, l.command)
	}
	everyWord := func(words []string) bool {
		for i := 1; i < len(words); i++ {
			for _, p := range patterns {
				if (!strings.HasPrefix(p.pattern, "re:") || i == 1) && p.leads(words[i:]) {
					return true
				}
			}
		}
		return false
	}
	for _, c := range commands {
		for _, dc := range append(directCommands(c, ""), directCmd{words: strings.Fields(c)}) {
			words := append([]string{"npx"}, dc.words...)
			got := launchedDeny(words, patterns)
			if want := everyWord(words); (got != nil) != want {
				t.Errorf("launchedDeny = %v, every word = %v: %q", got != nil, want, words)
			}
			if want := launchedDenyWordByWord(words, patterns); got != want {
				t.Errorf("launchedDeny = %s, word by word = %s: %q",
					denyName(got), denyName(want), words)
			}
		}
	}
}

// launchedDenyWordByWord is launchedDeny read the slow way, as it was first
// written: each entry tried anew at each word after the launcher's name it
// may start at (mayStartAt), the first word first. The one-pass read must
// find the same entry.
func launchedDenyWordByWord(words []string, patterns []deny) *deny {
	if !launches(words) {
		return nil
	}
	for i := 1; i < len(words); i++ {
		for j := range patterns {
			d := patterns[j]
			if strings.HasPrefix(d.pattern, "re:") {
				if i == 1 && d.leads(words[1:]) {
					return &patterns[j]
				}
				continue
			}
			if mayStartAt(d, words[i]) && d.leads(words[i:]) {
				return &patterns[j]
			}
		}
	}
	return nil
}

// mayStartAt reports whether a command whose first word is word may start
// with the entry: the word names the entry's tool, or holds the text
// match's first word.
func mayStartAt(d deny, word string) bool {
	if d.entry != nil && strings.EqualFold(commandName(ansiCWord(word)), d.entry.tool) {
		return true
	}
	return d.lead != nil && d.lead.MatchString(word)
}

// denyName is an entry's pattern for a test message, or none.
func denyName(d *deny) string {
	if d == nil {
		return "none"
	}
	return strconv.Quote(d.pattern)
}

// launchProbeEntries are entries of every shape the launcher read takes
// apart: rm's own reader, verbs past flags and `--`, gcloud tracks, a task
// tool, terraform's -destroy, required options with and without a value,
// phrases, text-only entries the reader refuses, and a regex.
var launchProbeEntries = []string{
	"rm -rf", "rm -rf /", "rm -r * /x", "rm --nope",
	"git push * --force", "git config --global", "git checkout -- .",
	"kubectl delete", "kubectl scale * --replicas=0 *", "gh api * -X DELETE *",
	"gcloud sql instances delete", "gcloud secrets delete",
	"rake db:drop", "terraform destroy", "tofu state rm",
	"psql * DROP TABLE *", "mysql * TRUNCATE *", `re:\bwipefs\b`,
}

// launchProbeWords are the words the probe commands are built from: tool
// names in every spelling, verbs, flags with and without a value, `--`,
// rm's options and operands, phrase words, quotes, brackets, $'...'
// strings, empty words and words with spaces in them.
var launchProbeWords = []string{
	"rm", "/bin/rm", `\rm`, "$'rm'", "Rm", "git", "kubectl", "KUBECTL", `"kubectl"`, "'gcloud'",
	"gcloud", "rake", "rails", "terraform", "tofu", "psql", "mysql", "gh", "docker",
	"delete", "Delete", "$'delete'", `$'\x64elete'`, "sql", "instances", "secrets", "beta",
	"push", "config", "checkout", "scale", "api", "db:drop", "apply", "destroy", "state",
	"-n", "prod", "--context", "--context=c", "--context=", "-C", "--force", "--global",
	"-f", "-r", "-rf", "-fr", "-R", "--recursive", "--rec", "/", "/x", ".", "x",
	"-X", "DELETE", "-XDELETE", "--replicas=0", "--replicas", "0", "00",
	"-destroy", "-destroy=false", "--destroy=true", "--", "-", "-c", "-cDROP", "$X", "X=1",
	"DROP", "TABLE", "'DROP", "TABLE'", "/*", "*/", "TRUNCATE", "drop table t", "kubectl delete",
	"'", `"`, "[", "]", ",", "''", `""`, "", " ", "wipefs", "a b",
}

// TestLaunchedDenyMatchesTheWordByWordRead: on commands built at random
// from launchProbeWords and the runs of words each probe entry names, the
// one-pass read finds the entry the word-by-word read finds. The seed is
// fixed, so a failure repeats.
func TestLaunchedDenyMatchesTheWordByWordRead(t *testing.T) {
	patterns := compilePatterns(launchProbeEntries)
	var runs [][]string
	for _, w := range launchProbeWords {
		runs = append(runs, []string{w})
	}
	for _, e := range launchProbeEntries {
		for _, group := range strings.Split(strings.TrimPrefix(e, "re:"), "*") {
			runs = append(runs, strings.Fields(group))
		}
	}
	rng := rand.New(rand.NewPCG(1, 2))
	const commands = 20000
	found := map[string]int{}
	for range commands {
		words := []string{"npx"}
		for range 1 + rng.IntN(10) {
			words = append(words, runs[rng.IntN(len(runs))]...)
		}
		got, want := launchedDeny(words, patterns), launchedDenyWordByWord(words, patterns)
		if got != want {
			t.Errorf("launchedDeny = %s, word by word = %s: %q",
				denyName(got), denyName(want), words)
		}
		found[denyName(want)]++
	}
	// The corpus must reach both verdicts and most entries, or it pins little.
	passed := found["none"]
	if passed < commands/10 || passed > commands*9/10 || len(found) < len(patterns)*3/4 {
		t.Errorf("the probe commands found %v, want both verdicts and most entries", found)
	}
}

// TestLaunchedDenyReadIsLinear pins the launcher read's cost: thousands of
// words behind a launcher that repeat a tool's name read in milliseconds,
// where a read anew from each word takes minutes. gcloud carries the most
// entries in real settings, so it gets sixty here; the other shapes keep a
// read going past flags, `--`, -destroy, a task tool's words, an option or
// a phrase that is never there, and rm's arguments.
func TestLaunchedDenyReadIsLinear(t *testing.T) {
	entries := append(slices.Clone(trancheEntries), launchProbeEntries...)
	for _, group := range []string{
		"sql instances", "compute instances", "container clusters", "pubsub topics",
		"storage buckets", "run services", "functions", "redis instances",
		"spanner instances", "dns record-sets",
	} {
		for _, verb := range []string{"delete", "update", "patch", "reset", "restart", "export"} {
			entries = append(entries, "gcloud "+group+" "+verb)
		}
	}
	patterns := compilePatterns(entries)
	tail := strings.Fields("gcloud beta sql instances delete db")
	start := time.Now()
	for _, shape := range []string{
		"gcloud", "gcloud --project", "gcloud beta", "git config", "git push", "kubectl --", "rake",
		"terraform -destroy", "psql -c", "rm", `"gcloud"`, "aws s3 rm", "gh api -X",
	} {
		unit := strings.Fields(shape)
		words := []string{"npx"}
		for range 5000 / len(unit) {
			words = append(words, unit...)
		}
		words = append(words, tail...)
		if launchedDeny(words, patterns) == nil {
			t.Errorf("npx and 5000 words of %q, then %q: want a denial",
				shape, strings.Join(tail, " "))
		}
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("the launcher read took %v over 13 commands of 5000 words, want under 2s", elapsed)
	}
}

// TestScriptDenyListReadsPastLaunchers: script-deny-list reads the same
// launchers, so an extra_patterns entry behind one denies with the advice
// for a wrapper, a settings entry asks the user to run it, and a command a
// launcher runs that no entry names passes.
func TestScriptDenyListReadsPastLaunchers(t *testing.T) {
	g := NewScriptDenyList(scriptDenyConfig([]string{"kubectl delete"}, []string{"rm -rf", `re:\bwipefs\b`}))
	tests := []struct {
		command, want string
	}{
		{"bundle exec rm -rf /tmp/x", "without the wrapper or launcher"},
		{"docker exec c wipefs -a /dev/sdb", "without the wrapper or launcher"},
		{"docker compose exec app rm -rf /data", "without the wrapper or launcher"},
		{"sudo npx rm -rf /tmp/x", "without the wrapper or launcher"},
		// The launcher's words read as the shell passes them.
		{"$'docker' exec db rm -rf /data", "without the wrapper or launcher"},
		{"do''cker exec db rm -rf /data", "without the wrapper or launcher"},
		{`\docker e"xe"c db rm -rf /data`, "without the wrapper or launcher"},
		{"kubectl -n prod exec pod -- kubectl delete pod x", askUserToRun},
		{"npx rimraf build", ""},
		{"docker exec app ls -la", ""},
		{"kubectl exec pod -- rm -r /tmp/x", ""},
	}
	for _, tt := range tests {
		d := g.Check(Input{Event: EventBash, Command: tt.command})
		switch {
		case tt.want == "" && d != nil:
			t.Errorf("%q: want a pass, got %s", tt.command, d.Reason)
		case tt.want != "" && d == nil:
			t.Errorf("%q: expected a denial", tt.command)
		case tt.want != "" && !strings.Contains(d.Reason, tt.want):
			t.Errorf("%q: reason %q\nwant it to hold %q", tt.command, d.Reason, tt.want)
		}
	}
}

// TestDirectDenyListReadsValuesAndComments pins the entry reader on whole
// commands: a glued value counts when it holds more than one word, an
// option letter past the first counts in a bundle of letters alone, and an
// SQL line comment is a gap up to its newline. These rows sit outside the
// frozen corpus, whose groups only grow at the end.
func TestDirectDenyListReadsValuesAndComments(t *testing.T) {
	g := NewDirectDenyList(config.Config{ClaudeDeny: trancheEntries})
	tests := []struct {
		command string
		deny    bool
	}{
		{"psql -dbackdrop -c 'select 1'", false},
		{"psql -cDROP", false},
		{"psql '-cDROP TABLE t'", true},
		{"psql '-cDROP TABLE t; select 1'", true},
		{`gh api -H"X-Req: $ID" repos/o/r`, false},
		{"docker compose down -t5v", false},
		{"gh api -iXDELETE repos/o/r", true},
		{"docker compose down -qv", true},
		{"psql -c 'DELETE -- x\nFROM t'", true},
		{"mysql -e 'DELETE # x\nFROM t'", true},
		{"psql -c 'DELETE --x FROM t'", false},
	}
	for _, tt := range tests {
		if got := g.Check(Input{Event: EventBash, Command: tt.command}) != nil; got != tt.deny {
			t.Errorf("deny = %v, want %v:\n%s", got, tt.deny, tt.command)
		}
	}
}

// TestDirectDenyListReadsDigitsAndNumbers pins a glued value after a digit
// option (psql's -1), an SQL statement set apart by \r, \f or \v, and an
// integer value written another way. These rows sit outside the frozen
// corpus too.
func TestDirectDenyListReadsDigitsAndNumbers(t *testing.T) {
	g := NewDirectDenyList(config.Config{ClaudeDeny: trancheEntries})
	tests := []struct {
		command string
		deny    bool
	}{
		{"psql -1c'DROP TABLE t'", true},
		{"psql -v x=1 -1c'DELETE FROM t'", true},
		{"psql -1c'select 1'", false},
		{`psql -c$'DROP\fTABLE\ft'`, true},
		{`psql -c$'DROP\rTABLE\rt'`, true},
		{`psql -c$'DELETE\vFROM\vt'`, true},
		{`psql -c$'select\f1'`, false},
		{"kubectl scale deploy x --replicas=00", true},
		{"kubectl scale deploy x --replicas 0x0", true},
		{"kubectl scale deploy x --replicas=01", false},
	}
	for _, tt := range tests {
		if got := g.Check(Input{Event: EventBash, Command: tt.command}) != nil; got != tt.deny {
			t.Errorf("deny = %v, want %v:\n%s", got, tt.deny, tt.command)
		}
	}
}

// TestDirectDenyListText pins the deny text: the entry, why every form is
// denied, and the ask to hand the command to the user.
func TestDirectDenyListText(t *testing.T) {
	g := NewDirectDenyList(config.Config{ClaudeDeny: []string{"kubectl delete"}})
	d := g.Check(Input{Event: EventBash, Command: "kubectl -n prod delete pod x"})
	if d == nil {
		t.Fatal("expected a denial")
	}
	want := `belt[direct-deny-list]: this runs "kubectl delete" from the Bash deny list ` +
		`(read past global flags and wrappers). It can delete or change state that may not come back, ` +
		`so every form of it is denied: a script, a wrapper or an alias too. ` +
		`Ask the user to run it; they can type ! <command> in the prompt.`
	if d.Guard != DirectDenyListID || d.Reason != want {
		t.Errorf("denial = %s %q\nwant %q", d.Guard, d.Reason, want)
	}
}

// TestDirectDenyListReadsTheSettingsListOnly: extra_patterns stay with
// script-deny-list, and with no settings entries (claude_settings.enabled:
// false leaves cfg.ClaudeDeny empty) the guard has nothing to deny.
func TestDirectDenyListReadsTheSettingsListOnly(t *testing.T) {
	cfg := scriptDenyConfig(nil, []string{"rm -rf", "kubectl delete"})
	g := NewDirectDenyList(cfg)
	for _, c := range []string{"rm -rf /tmp/x", "kubectl delete pod x"} {
		if d := g.Check(Input{Event: EventBash, Command: c}); d != nil {
			t.Errorf("an extra_patterns entry denies %q: %s", c, d.Reason)
		}
	}
}

// TestDirectDenyListSoftMode: soft mode lets the call through with one
// warn event under the soft title, naming the entry.
func TestDirectDenyListSoftMode(t *testing.T) {
	cfg := config.Config{
		ClaudeDeny: []string{"kubectl delete"},
		Guards:     map[string]config.Toggle{DirectDenyListID: {Mode: config.ModeSoft}},
	}
	var titles, messages []string
	g := &DirectDenyList{cfg: cfg, emit: func(_, level, title, message string, _ map[string]string) {
		titles = append(titles, level+": "+title)
		messages = append(messages, message)
	}}
	if d := g.Check(Input{Event: EventBash, Command: "sudo kubectl delete pod x"}); d != nil {
		t.Fatalf("soft mode must allow, got %+v", d)
	}
	if !slices.Equal(titles, []string{"warn: direct-deny-list (soft)"}) {
		t.Fatalf("want one soft warn, got %v", titles)
	}
	if !strings.Contains(messages[0], `"kubectl delete"`) {
		t.Errorf("the warn should name the entry: %s", messages[0])
	}
}

// TestDirectDenyListRunsBeforeScriptDenyList: a deny-listed command behind a
// wrapper gets direct-deny-list's text, and a git command a git guard owns
// keeps that guard's.
func TestDirectDenyListRunsBeforeScriptDenyList(t *testing.T) {
	var ids []string
	for _, g := range All(config.Config{}) {
		ids = append(ids, g.ID())
	}
	at := slices.Index(ids, DirectDenyListID)
	if at < 0 || ids[at+1] != ScriptDenyListID || at < slices.Index(ids, GitDiscardID) ||
		at < slices.Index(ids, GitPushMainID) {
		t.Fatalf("direct-deny-list must run after the git guards, right before script-deny-list: %v", ids)
	}
	cfg := config.Config{ClaudeDeny: []string{"kubectl delete"}}
	d := Run(Input{Event: EventBash, Command: "sudo kubectl delete pod x"}, cfg)
	if d == nil || d.Guard != DirectDenyListID {
		t.Fatalf("want a direct-deny-list denial, got %+v", d)
	}
}

// TestScriptDenyListAsksForSettingsEntries: inside a script, an entry from
// the settings list asks the user to run the command, since the direct form
// is denied too, and an extra_patterns entry keeps the advice to run it
// directly.
func TestScriptDenyListAsksForSettingsEntries(t *testing.T) {
	g := NewScriptDenyList(scriptDenyConfig([]string{"kubectl delete"}, []string{"rm -rf"}))
	tests := []struct {
		command, want, never string
	}{
		{"bash -c 'kubectl delete pod x'", askUserToRun, "permission system can evaluate"},
		{"bash -c 'rm -rf /tmp/x'", "Run the blocked operation as a direct command", askUserToRun},
		{"script -q /dev/null kubectl delete pod x", askUserToRun, "without the wrapper"},
		{"$KUBECTL delete pod x", askUserToRun, "Write the command name out"},
		{"$RM -rf /tmp/x", "Write the command name out", askUserToRun},
	}
	for _, tt := range tests {
		d := g.Check(Input{Event: EventBash, Command: tt.command})
		if d == nil {
			t.Errorf("%q: expected a denial", tt.command)
			continue
		}
		if !strings.Contains(d.Reason, tt.want) || strings.Contains(d.Reason, tt.never) {
			t.Errorf("%q: reason %q\nwant it to hold %q and not %q", tt.command, d.Reason, tt.want, tt.never)
		}
	}
}
