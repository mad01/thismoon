// Package queue provides a file-backed reindex queue.
//
// Post-merge hooks append repo paths to a queue file using O_APPEND (atomic
// for writes under PIPE_BUF). A drain step claims the queue atomically via
// rename, deduplicates, and returns the list of repos to reindex.
package queue

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/services/csl"
)

const fileName = "reindex.queue"

// DefaultPath returns the reindex queue inside csl's state directory.
func DefaultPath() (string, error) {
	return csl.StatePath(fileName)
}

// Enqueue appends repoPath to the queue file.
// Safe to call from concurrent processes — O_APPEND writes under PIPE_BUF
// are atomic on POSIX systems.
func Enqueue(queuePath, repoPath string) error {
	if err := os.MkdirAll(filepath.Dir(queuePath), 0o755); err != nil {
		return fmt.Errorf("create queue dir: %w", err)
	}
	f, err := os.OpenFile(queuePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open queue %s: %w", queuePath, err)
	}
	_, writeErr := fmt.Fprintln(f, repoPath)
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("write to queue: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close queue: %w", closeErr)
	}
	return nil
}

// Claim atomically moves the queue file to a claimed path and returns the
// deduplicated list of repo paths. Subsequent Enqueue calls start a new file.
// Returns ("", nil, nil) if the queue does not exist or is empty.
func Claim(queuePath string) (claimedPath string, repos []string, err error) {
	claimed := queuePath + ".claimed." + strconv.Itoa(os.Getpid())
	if err := os.Rename(queuePath, claimed); err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("claim queue: %w", err)
	}
	repos, err = readAndDedup(claimed)
	if err != nil {
		return claimed, nil, err
	}
	if len(repos) == 0 {
		_ = os.Remove(claimed)
		return "", nil, nil
	}
	return claimed, repos, nil
}

// Release removes the claimed file after successful processing.
func Release(claimedPath string) error {
	if claimedPath == "" {
		return nil
	}
	return os.Remove(claimedPath)
}

// PendingCount returns the number of unique repo paths in the queue
// without claiming it. Returns 0 if the queue does not exist.
func PendingCount(queuePath string) int {
	repos, err := readAndDedup(queuePath)
	if err != nil {
		return 0
	}
	return len(repos)
}

func readAndDedup(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read queue %s: %w", path, err)
	}
	defer f.Close()

	seen := make(map[string]struct{})
	var repos []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if _, ok := seen[line]; !ok {
			seen[line] = struct{}{}
			repos = append(repos, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan queue %s: %w", path, err)
	}
	return repos, nil
}
