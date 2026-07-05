package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/t-man/internal/platform/launchd"
	"github.com/mad01/thismoon/tools/t-man/internal/service"
	"github.com/spf13/cobra"
)

var (
	logsFollow bool
	logsStdout bool
	logsStderr bool
	logsSource string
	logsLines  int
)

// followPollInterval is how often follow mode checks the files for new data
const followPollInterval = 500 * time.Millisecond

// logsCmd represents the logs command
var logsCmd = &cobra.Command{
	Use:   "logs <service-name>",
	Short: "View service logs",
	Long: `View logs for a service.

By default both stdout and stderr are shown, with a tail-style header
(==> source <==) marking which file each block came from. Use --stdout or
--stderr to show a single stream, or --source to view a named extra log
registered with 'add --extra-log' (e.g. --source sandbox).`,
	Example: `  # Last 50 lines of stdout + stderr
  t-man logs myapp

  # Only stderr, following new output
  t-man logs myapp --stderr -f

  # A named extra log (registered via: add --extra-log sandbox=/path/to.log)
  t-man logs myapp --source sandbox`,
	Args: cobra.ExactArgs(1),
	RunE: runLogs,
}

// logsSandboxCmd represents the logs sandbox subcommand
var logsSandboxCmd = &cobra.Command{
	Use:   "sandbox [service-name]",
	Short: "View sandbox logs",
	Long: `View sandbox logs registered as extra logs.

Collects extra log sources named "sandbox" or "sandbox-*" — by convention the
seatbelt denial ledger and notification mirror written by a sandbox watcher.
With a service name only that service's sandbox sources are shown; without
one, sandbox sources from all managed services are combined, and duplicate
file paths (the same ledger registered on several services) are shown once.`,
	Example: `  # All sandbox logs across the fleet
  t-man logs sandbox

  # Follow one service's sandbox logs
  t-man logs sandbox speak-tts -f`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSandboxLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.AddCommand(logsSandboxCmd)

	logsCmd.PersistentFlags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.PersistentFlags().
		IntVarP(&logsLines, "lines", "n", 50, "Number of lines to show from the end")
	logsCmd.Flags().BoolVar(&logsStdout, "stdout", false, "Show only stdout")
	logsCmd.Flags().BoolVar(&logsStderr, "stderr", false, "Show only stderr")
	logsCmd.Flags().
		StringVar(&logsSource, "source", "", "Show a single log source: stdout, stderr, or an extra log name")
	logsCmd.MarkFlagsMutuallyExclusive("stdout", "stderr", "source")
}

// logSource is one log file to read, identified by its source name
type logSource struct {
	name string
	path string
}

func runLogs(cmd *cobra.Command, args []string) error {
	if err := checkSudo(); err != nil {
		return err
	}

	serviceName := args[0]

	manager := launchd.NewManager(GetVersion(), !daemonMode)

	svc, err := manager.Get(getContext(), serviceName)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	sources, err := resolveLogSources(svc, logsSource, logsStdout, logsStderr)
	if err != nil {
		return err
	}

	w := os.Stdout
	printer := newSourcePrinter(w, len(sources) > 1)

	if err := tailSources(printer, sources, logsLines); err != nil {
		return err
	}

	if logsFollow {
		fmt.Fprintf(os.Stderr, "[Following — press Ctrl+C to exit]\n")
		return followSources(printer, sources)
	}

	return nil
}

func runSandboxLogs(cmd *cobra.Command, args []string) error {
	if err := checkSudo(); err != nil {
		return err
	}

	manager := launchd.NewManager(GetVersion(), !daemonMode)

	var sources []logSource
	if len(args) == 1 {
		svc, err := manager.Get(getContext(), args[0])
		if err != nil {
			return fmt.Errorf("failed to get service: %w", err)
		}
		sources = sandboxSources([]*service.Definition{svc}, false)
		if len(sources) == 0 {
			return fmt.Errorf(
				"service %q has no sandbox logs registered (register with: t-man add --extra-log sandbox=PATH)",
				args[0],
			)
		}
	} else {
		svcs, err := manager.List(getContext())
		if err != nil {
			return fmt.Errorf("failed to list services: %w", err)
		}
		sources = sandboxSources(svcs, true)
		if len(sources) == 0 {
			return fmt.Errorf("no sandbox logs registered on any service (register with: t-man add --extra-log sandbox=PATH)")
		}
	}

	printer := newSourcePrinter(os.Stdout, len(sources) > 1)
	if err := tailSources(printer, sources, logsLines); err != nil {
		return err
	}
	if logsFollow {
		fmt.Fprintf(os.Stderr, "[Following — press Ctrl+C to exit]\n")
		return followSources(printer, sources)
	}
	return nil
}

// isSandboxSource reports whether an extra-log source name follows the
// sandbox naming convention ("sandbox" or "sandbox-*")
func isSandboxSource(name string) bool {
	return name == "sandbox" || strings.HasPrefix(name, "sandbox-")
}

// sandboxSources collects the sandbox extra logs of the given services,
// de-duplicated by file path. Fleet mode labels entries by path — the same
// ledger is typically registered on several services, so a source name would
// arbitrarily attribute it to whichever service came first.
func sandboxSources(svcs []*service.Definition, fleet bool) []logSource {
	seen := make(map[string]bool)
	var sources []logSource
	for _, svc := range svcs {
		for _, name := range sortedKeys(svc.ExtraLogs) {
			if !isSandboxSource(name) {
				continue
			}
			path := svc.ExtraLogs[name]
			if seen[path] {
				continue
			}
			seen[path] = true
			label := name
			if fleet {
				label = displayPath(path)
			}
			sources = append(sources, logSource{name: label, path: path})
		}
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].name < sources[j].name })
	return sources
}

// displayPath shortens a home-prefixed path to ~/... for display
func displayPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+"/") {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// resolveLogSources maps the flag selection to the ordered list of log files
// to read. With no selection both standard streams are returned.
func resolveLogSources(
	svc *service.Definition,
	source string,
	stdoutOnly, stderrOnly bool,
) ([]logSource, error) {
	stdout := logSource{name: "stdout", path: svc.StandardOutPath}
	stderr := logSource{name: "stderr", path: svc.StandardErrPath}

	switch {
	case source == "stdout" || stdoutOnly:
		if stdout.path == "" {
			return nil, fmt.Errorf("no stdout log configured for this service")
		}
		return []logSource{stdout}, nil
	case source == "stderr" || stderrOnly:
		if stderr.path == "" {
			return nil, fmt.Errorf("no stderr log configured for this service")
		}
		return []logSource{stderr}, nil
	case source != "":
		path, ok := svc.ExtraLogs[source]
		if !ok {
			return nil, fmt.Errorf(
				"unknown log source %q (available: %s)",
				source,
				strings.Join(availableSources(svc), ", "),
			)
		}
		return []logSource{{name: source, path: path}}, nil
	}

	var sources []logSource
	if stdout.path != "" {
		sources = append(sources, stdout)
	}
	if stderr.path != "" {
		sources = append(sources, stderr)
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("no log files configured for this service")
	}
	return sources, nil
}

// availableSources lists the selectable source names for a service
func availableSources(svc *service.Definition) []string {
	sources := []string{"stdout", "stderr"}
	return append(sources, sortedKeys(svc.ExtraLogs)...)
}

// sortedKeys returns the map keys in sorted order
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sourcePrinter writes log lines, emitting a tail-style "==> name <==" header
// whenever output switches to a different source. Headers are skipped when
// only a single source is shown.
type sourcePrinter struct {
	w           io.Writer
	withHeaders bool
	current     string
}

func newSourcePrinter(w io.Writer, withHeaders bool) *sourcePrinter {
	return &sourcePrinter{w: w, withHeaders: withHeaders}
}

func (p *sourcePrinter) printLine(source, line string) {
	if p.withHeaders && source != p.current {
		if p.current != "" {
			fmt.Fprintln(p.w)
		}
		fmt.Fprintf(p.w, "==> %s <==\n", source)
		p.current = source
	}
	fmt.Fprintln(p.w, line)
}

// tailSources prints the last N lines of each source. Missing files are
// skipped when multiple sources are shown; a single missing file is an error.
func tailSources(p *sourcePrinter, sources []logSource, lines int) error {
	existing := 0
	for _, src := range sources {
		srcLines, err := lastNLines(src.path, lines)
		if err != nil {
			if os.IsNotExist(err) && len(sources) > 1 {
				continue
			}
			if os.IsNotExist(err) {
				return fmt.Errorf("log file does not exist: %s", src.path)
			}
			return fmt.Errorf("failed to read log file %s: %w", src.path, err)
		}
		existing++
		for _, line := range srcLines {
			p.printLine(src.name, line)
		}
	}
	if existing == 0 {
		return fmt.Errorf("no log files exist yet for the selected sources")
	}
	return nil
}

// lastNLines returns up to the last n lines of the file at path
func lastNLines(path string, n int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

// tailState tracks the read position into one followed file
type tailState struct {
	src    logSource
	offset int64
	buf    []byte // partial line carried between polls
}

// followSources polls the source files and prints lines as they are appended,
// like tail -f. Truncated files restart from the beginning; files that do not
// exist yet are picked up once created.
func followSources(p *sourcePrinter, sources []logSource) error {
	states := make([]*tailState, len(sources))
	for i, src := range sources {
		st := &tailState{src: src}
		if info, err := os.Stat(src.path); err == nil {
			st.offset = info.Size() // tail already printed the existing content
		}
		states[i] = st
	}

	for {
		for _, st := range states {
			if err := st.drain(p); err != nil {
				return err
			}
		}
		time.Sleep(followPollInterval)
	}
}

// drain reads newly appended data and prints any complete lines
func (st *tailState) drain(p *sourcePrinter) error {
	info, err := os.Stat(st.src.path)
	if err != nil {
		if os.IsNotExist(err) {
			st.offset = 0
			return nil
		}
		return fmt.Errorf("failed to stat log file %s: %w", st.src.path, err)
	}

	size := info.Size()
	if size < st.offset {
		// truncated (e.g. log rotation) — start over
		st.offset = 0
		st.buf = nil
	}
	if size == st.offset {
		return nil
	}

	file, err := os.Open(st.src.path)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", st.src.path, err)
	}
	defer file.Close()

	if _, err := file.Seek(st.offset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek log file %s: %w", st.src.path, err)
	}

	data, err := io.ReadAll(io.LimitReader(file, size-st.offset))
	if err != nil {
		return fmt.Errorf("failed to read log file %s: %w", st.src.path, err)
	}
	st.offset += int64(len(data))

	st.buf = append(st.buf, data...)
	for {
		idx := bytes.IndexByte(st.buf, '\n')
		if idx < 0 {
			break
		}
		p.printLine(st.src.name, string(st.buf[:idx]))
		st.buf = st.buf[idx+1:]
	}
	return nil
}
