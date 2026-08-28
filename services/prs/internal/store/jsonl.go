package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// prepareLog ensures workdir exists and returns the log path.
func prepareLog(workdir, name string) (string, error) {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return "", fmt.Errorf("create workdir: %w", err)
	}
	return filepath.Join(workdir, name), nil
}

// scanLog reads the JSONL log and resolves the newest record per key:
// greater fetched_at wins, a tie goes to the later line. A missing file is
// an empty log. A torn final line — no trailing newline and unparseable, the
// footprint of an append cut short by a crash — is truncated away; a
// parse error anywhere else is fatal and names the line, since guessing
// which records to keep would hide the corruption.
func scanLog(path string) (map[string]RepoState, int, error) {
	repos := map[string]RepoState{}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return repos, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("read log: %w", err)
	}

	lines := bytes.Split(raw, []byte("\n"))
	// A trailing newline yields one empty final element; drop it.
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	count := 0
	for i, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var st RepoState
		if err := json.Unmarshal(line, &st); err != nil {
			if i == len(lines)-1 && !bytes.HasSuffix(raw, []byte("\n")) {
				if terr := truncateTail(path, raw, line); terr != nil {
					return nil, 0, terr
				}
				break
			}
			return nil, 0, fmt.Errorf(
				"prs: log %s line %d is corrupt (delete the file and refresh): %w",
				path, i+1, err,
			)
		}
		count++
		prev, ok := repos[st.Key()]
		if !ok || !st.FetchedAt.Before(prev.FetchedAt) {
			repos[st.Key()] = st
		}
	}
	return repos, count, nil
}

// truncateTail drops a torn final line left by an interrupted append.
func truncateTail(path string, raw, torn []byte) error {
	if err := os.WriteFile(path, raw[:len(raw)-len(torn)], 0o644); err != nil {
		return fmt.Errorf("truncate torn log line: %w", err)
	}
	return nil
}

// appendLog writes one record as one line; the append is the atomic unit.
func appendLog(path string, st RepoState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("append log: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close log: %w", err)
	}
	return nil
}

// shouldCompact reports whether the log has accumulated enough superseded
// lines to be worth rewriting: past a fixed floor, and more than four lines
// per live key.
func shouldCompact(lines, keys int) bool {
	return lines > 100 && lines > 4*keys
}

// compactLog rewrites the log as one line per key, oldest fetched_at first,
// via a temp file and rename. Only New calls it, before serve starts
// appending, so the single-writer rule holds.
func compactLog(path string, repos map[string]RepoState) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("compact log: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	states := make([]RepoState, 0, len(repos))
	for _, st := range repos {
		states = append(states, st)
	}
	slices.SortFunc(states, func(a, b RepoState) int {
		if c := a.FetchedAt.Compare(b.FetchedAt); c != 0 {
			return c
		}
		return strings.Compare(a.Key(), b.Key())
	})

	w := bufio.NewWriter(tmp)
	for _, st := range states {
		raw, err := json.Marshal(st)
		if err != nil {
			_ = tmp.Close()
			return fmt.Errorf("compact log: marshal: %w", err)
		}
		if _, err := w.Write(append(raw, '\n')); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("compact log: write: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("compact log: flush: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("compact log: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("compact log: close: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("compact log: rename: %w", err)
	}
	return nil
}
