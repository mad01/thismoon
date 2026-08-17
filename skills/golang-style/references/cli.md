# CLI patterns (cobra)

Every tool here is a cobra CLI with the same skeleton. Backed by the [cobra docs](https://github.com/spf13/cobra) and the repo CLIs.

## Thin `main`, `cli.Execute()`, root command

`cmd/<tool>/main.go` does nothing but call `Execute` and turn an error into an exit code:

```go
func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "worklog: "+err.Error())
		os.Exit(1)
	}
}
```

`internal/cli` builds the command tree. The root sets `SilenceUsage` and `SilenceErrors` so cobra doesn't print usage or the error itself — error printing happens once in `main` (see `errors.md`). From `worklog/internal/cli/cli.go`:

```go
// Execute runs the root command.
func Execute() error { return root().Execute() }

func root() *cobra.Command {
	c := &cobra.Command{
		Use:           "worklog",
		Short:         "Resumable, ticket/topic-keyed cross-session work state",
		Version:       buildinfo.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.AddCommand(versionCmd(), newCmd(), checkpointCmd(), listCmd(), showCmd(), searchCmd(), statusCmd(), pathCmd(), scanCmd(), mcpCmd())
	return c
}
```

## Subcommands as builder functions returning `*cobra.Command`

Each subcommand is a function that declares its flags as locals, binds them, and returns the command. Use `RunE` (not `Run`) so the command returns an error up to `Execute`:

```go
func scanCmd() *cobra.Command {
	var since string
	c := &cobra.Command{
		Use:   "scan",
		Short: "Digest recent Claude session transcripts as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := parseSince(since)
			if err != nil {
				return err
			}
			sessions, err := scan.Scan("", d, time.Now())
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(sessions)
		},
	}
	c.Flags().StringVar(&since, "since", "14d", "window: Nd (days) or a Go duration like 336h")
	return c
}
```

Keep the `RunE` body thin: parse flags, call into `internal/` packages, return their error. Set `Args` (`cobra.NoArgs`, `cobra.ExactArgs(1)`, …) to validate positional arguments instead of checking by hand.

## Flags

- Bind to a local with `c.Flags().StringVar(&v, "name", default, "usage")`. Persistent flags (shared by subcommands) go on the root with `PersistentFlags()`.
- An env fallback is read in code, not via a config framework — e.g. `os.Getenv("WORKLOG_DIR")` inside `store.DefaultRoot()`.

## Build metadata via ldflags, `version -o json`

Build metadata isn't a per-tool `var Version` any more. Tools link a shared `buildinfo` package holding four variables: `Version` (the short sha, or `vX.Y.Z-<sha>` for a release build), `Commit` (the full sha), `Tag` (the last release tag, e.g. `worklog/v0.3.1`), and `BuildTime` (UTC, RFC 3339). All four are injected at link time, and `buildinfo.Get()` fills whatever the ldflags left empty from the toolchain's vcs stamps — so a `go install`ed binary still reports a real commit.

In the thismoon monorepo the ldflags live in one Makefile fragment at the repo root. A component names itself and includes it:

```make
COMPONENT := worklog
include ../../buildinfo.mk

build:
	go build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(BIN) ./cmd/worklog
```

A standalone repo mirrors the shape with its own package rather than importing another repo's — ralph carries `github.com/mad01/ralph/internal/buildinfo`.

The output is the cross-tool convention:

- `<tool> version` prints the bare version token and nothing else. status parses it to compare a running service against its installed binary, so a `version` that prints a banner or a prefix disables drift detection for that tool.
- `<tool> version -o json` prints the 2-space-indented object, all four keys always present, `""` for anything the build couldn't determine:

```json
{
  "version": "9f3c1ab",
  "commit": "9f3c1abf20e4c7d1b8a5e6003f2c9d47a1b6e850",
  "tag": "worklog/v0.3.1",
  "build_time": "2026-08-13T19:40:02Z"
}
```

`ralph doctor` probes every installed binary with that command and dates it by `commit` (falling back to `version`), which is how it tells a stale binary from a fresh one. Web services serve the identical object from `GET /version` — see `http.md`.

## Output discipline

- Real output goes to `os.Stdout` (often `json.NewEncoder(os.Stdout)`); diagnostics and the final error go to `os.Stderr`.
- CLI tools print with `fmt`; long-running servers log with `log.Printf` (see `http.md`). No structured logger.
