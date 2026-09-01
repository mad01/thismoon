package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/humanizer/internal/backend"
	"github.com/mad01/thismoon/tools/humanizer/internal/goscan"
	"github.com/mad01/thismoon/tools/humanizer/internal/judge"
	"github.com/mad01/thismoon/tools/humanizer/internal/rules"
)

var (
	scanGo       bool
	scanDetect   bool
	scanHolistic bool
	scanKinds    []string
)

var scanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Extract the prose out of source files and scan it",
	Long: `Extract the human-facing prose from Go source and print it as plain
text, one block per site, tagged with file:line. Feed it to 'humanizer
detect' or scan it in place with --detect.

--go selects the Go extractor, the only one so far. It reads doc comments,
the cobra Use/Short/Long/Example fields, MCP tool Description strings, the
message arguments of fmt.Errorf, errors.New and http.Error, and the text in
jsonschema struct tags. Restrict it with --kind (doc, cobra, mcp, error,
schema).

A directory argument is walked recursively. Dot-directories and vendor,
node_modules and testdata are skipped, as are generated files; test files
are read like any other source. The path defaults to the current directory.

--detect runs the vale span rules over each file's prose and reports every
finding against the line of Go source it came from. --holistic sends each
file's prose to the LLM judge; with no backend configured it prints a note
and the rest of the scan continues.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runScan,
}

func init() {
	scanCmd.Flags().
		BoolVar(&scanGo, "go", false, "extract from Go source (the only extractor so far)")
	scanCmd.Flags().
		BoolVar(&scanDetect, "detect", false, "run the vale span rules over the extracted prose")
	scanCmd.Flags().
		BoolVar(&scanHolistic, "holistic", false, "send each file's prose to the LLM judge")
	scanCmd.Flags().
		StringSliceVar(&scanKinds, "kind", nil, "restrict extraction to this kind (doc|cobra|mcp|error|schema, repeatable)")
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	if !scanGo {
		return errors.New("scan: --go is required; it selects the Go source extractor")
	}
	opts, err := scanOptions(scanKinds)
	if err != nil {
		return err
	}
	path := "."
	if len(args) == 1 {
		path = args[0]
	}
	files, err := goscan.Scan(path, opts)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(files) == 0 {
		fmt.Fprintln(out, "no prose extracted")
		return nil
	}
	if !scanDetect && !scanHolistic {
		printBlocks(out, files)
		return nil
	}
	return reportScan(cmd, files)
}

// scanOptions turns the --kind values into extraction options.
func scanOptions(names []string) (goscan.Options, error) {
	var kinds []goscan.Kind
	for _, name := range names {
		kind, ok := goscan.ParseKind(strings.TrimSpace(name))
		if !ok {
			return goscan.Options{}, fmt.Errorf(
				"scan: unknown kind %q; want one of %s",
				name,
				kindNames(),
			)
		}
		kinds = append(kinds, kind)
	}
	return goscan.Options{Kinds: kinds}, nil
}

func kindNames() string {
	names := make([]string, 0, len(goscan.Kinds()))
	for _, k := range goscan.Kinds() {
		names = append(names, string(k))
	}
	return strings.Join(names, ", ")
}

// printBlocks writes the extracted prose as plain text: a header per file,
// then every block behind its own file:line line.
func printBlocks(w io.Writer, files []goscan.File) {
	for i, f := range files {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "==> %s <==\n", f.Path)
		for _, b := range f.Blocks {
			fmt.Fprintf(w, "\n%s:%d  [%s] %s\n", b.File, b.Line, b.Kind, b.Label)
			fmt.Fprintln(w, b.Text)
		}
	}
}

// reportScan runs the requested passes file by file. Each file's blocks go
// in as one document, so vale runs once per file and the findings map back
// through the document's line index.
func reportScan(cmd *cobra.Command, files []goscan.File) error {
	out := cmd.OutOrStdout()
	llm, err := scanBackend(cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	if !scanDetect && llm == nil {
		printBlocks(out, files) // holistic was skipped: fall back to the dump
		return nil
	}
	total := rules.NewSummary()
	for _, f := range files {
		doc := goscan.Render(f.Blocks)
		fmt.Fprintf(out, "==> %s <==\n", f.Path)
		if scanDetect {
			if err := detectDoc(cmd, f.Path, doc, &total); err != nil {
				return err
			}
		}
		if llm != nil {
			judgeDoc(cmd.Context(), out, llm, doc)
		}
		fmt.Fprintln(out)
	}
	if scanDetect {
		fmt.Fprintf(out, "%d findings in %d files (error=%d warning=%d suggestion=%d)\n",
			total.Total, len(files), total.BySeverity["error"],
			total.BySeverity["warning"], total.BySeverity["suggestion"])
	}
	return nil
}

// scanBackend resolves the judge backend once for the whole scan. A missing
// backend is not fatal: --holistic is the optional pass, so it degrades to
// a note on stderr.
func scanBackend(errOut io.Writer) (backend.Backend, error) {
	if !scanHolistic {
		return nil, nil
	}
	llm, err := backend.Select("", "")
	if err != nil {
		if errors.Is(err, backend.ErrNoBackend) {
			fmt.Fprintf(errOut, "scan: skipping --holistic, %v\n", err)
			return nil, nil
		}
		return nil, err
	}
	return llm, nil
}

// detectDoc runs the span rules over one file's prose document and prints
// the findings against their Go source lines.
func detectDoc(cmd *cobra.Command, path string, doc goscan.Doc, total *rules.Summary) error {
	findings, err := rules.Detect(cmd.Context(), doc.Text, rules.DetectOptions{})
	if err != nil {
		notInstalled := &rules.ValeNotInstalledError{}
		if errors.As(err, &notInstalled) {
			fmt.Fprintln(cmd.ErrOrStderr(), err)
		}
		return err
	}
	out := cmd.OutOrStdout()
	if len(findings) == 0 {
		fmt.Fprintln(out, "no findings")
		return nil
	}
	for _, f := range findings {
		total.Add(f.Severity, f.Category, f.RuleID)
		fmt.Fprintf(out, "%s  %s:%d  [%s]  %s: %s\n",
			severityBadge(f.Severity), path, doc.SourceLine(f.Line), f.RuleID, f.Match, f.Message)
	}
	return nil
}

// judgeDoc prints the LLM verdict for one file's prose. A judge failure is
// reported and the scan moves on: the deterministic pass is the one that
// has to complete.
func judgeDoc(ctx context.Context, out io.Writer, llm backend.Backend, doc goscan.Doc) {
	ctx, cancel := context.WithTimeout(ctx, judgeTimeout)
	defer cancel()
	v, err := judge.Run(ctx, llm, doc.Text)
	if err != nil {
		fmt.Fprintf(out, "judge: failed: %v\n", err)
		return
	}
	fmt.Fprintf(out, "judge: %s (confidence %.2f): %s\n", v.Verdict, v.Confidence, v.Summary)
}
