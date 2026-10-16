package store_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/present/internal/store"
	"github.com/mad01/thismoon/services/present/internal/store/storetest"
)

// wrappedLimit is a cap no conformance page comes near, so the wrapped
// store must behave exactly like the raw one.
const wrappedLimit = 1 << 20

func newFS(t *testing.T, dir string) *store.FS {
	t.Helper()
	fs, err := store.NewFS(dir)
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	return fs
}

func TestFSEditConformance(t *testing.T) {
	storetest.RunEditConformance(t, func(t *testing.T) store.Store {
		return newFS(t, t.TempDir())
	})
}

// TestWrappedFSEditConformance runs the contract through the wrappers a
// shared instance stacks, so an edit through them reaches the store with
// its semantics intact.
func TestWrappedFSEditConformance(t *testing.T) {
	storetest.RunEditConformance(t, func(t *testing.T) store.Store {
		fs := newFS(t, t.TempDir())
		return store.WithSizeLimit(store.WithoutExpired(fs, time.Now), wrappedLimit)
	})
}

// editHelperEnv carries the job a re-executed test binary runs as one
// writer process of TestEditAcrossProcesses.
const editHelperEnv = "PRESENT_STORE_EDIT_HELPER"

// editJob is one writer process's work: append Edits tokens named after
// Name to page ID in the workdir Dir.
type editJob struct {
	Dir   string `json:"dir"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Edits int    `json:"edits"`
}

// TestEditAcrossProcesses runs two writer processes over one workdir, the
// way the MCP server, serve, and the CLI share it, and requires every edit
// of both to land: the page lock orders processes, not just goroutines.
func TestEditAcrossProcesses(t *testing.T) {
	const writers, edits = 2, 25
	dir := t.TempDir()
	st := newFS(t, dir)
	id := storetest.NewTokenPage(t, st)

	var want []string
	procs := make([]*writerProcess, writers)
	for w := range writers {
		job := editJob{Dir: dir, ID: id, Name: fmt.Sprintf("p%d", w), Edits: edits}
		for i := range edits {
			want = append(want, tokenName(job.Name, i))
		}
		procs[w] = startWriter(t, job)
	}
	// Closing stdin is the starting gun: both processes are up and
	// waiting, so their edits overlap.
	for w, p := range procs {
		if err := p.stdin.Close(); err != nil {
			t.Fatalf("start writer %d: %v", w, err)
		}
	}
	for w, p := range procs {
		if err := p.cmd.Wait(); err != nil {
			t.Fatalf("writer %d: %v\n%s", w, err, p.out.String())
		}
	}
	storetest.CheckTokens(t, st, id, want)
}

// TestEditHelperProcess is the writer TestEditAcrossProcesses starts. Run
// directly it has no job and skips.
func TestEditHelperProcess(t *testing.T) {
	raw := os.Getenv(editHelperEnv)
	if raw == "" {
		t.Skip("runs only as a writer process of TestEditAcrossProcesses")
	}
	var job editJob
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if _, err := io.ReadAll(os.Stdin); err != nil {
		t.Fatalf("wait for the start: %v", err)
	}
	st := newFS(t, job.Dir)
	for i := range job.Edits {
		if _, err := st.Edit(t.Context(), job.ID, storetest.AppendToken(tokenName(job.Name, i))); err != nil {
			t.Fatalf("edit %d: %v", i, err)
		}
	}
}

func tokenName(writer string, i int) string { return fmt.Sprintf("%s-%d", writer, i) }

// writerProcess is a started writer waiting for its stdin to close.
type writerProcess struct {
	cmd   *exec.Cmd
	out   *strings.Builder
	stdin io.WriteCloser
}

func startWriter(t *testing.T, job editJob) *writerProcess {
	t.Helper()
	spec, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestEditHelperProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), editHelperEnv+"="+string(spec))
	out := &strings.Builder{}
	cmd.Stdout, cmd.Stderr = out, out
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start writer: %v", err)
	}
	t.Cleanup(func() {
		// A test that failed before waiting leaves the writer blocked on
		// stdin; kill it so it does not outlive the test.
		if cmd.ProcessState == nil {
			if err := cmd.Process.Kill(); err != nil {
				t.Logf("kill writer: %v", err)
			}
		}
	})
	return &writerProcess{cmd: cmd, out: out, stdin: stdin}
}
