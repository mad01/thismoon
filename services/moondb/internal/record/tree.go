package record

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/services/moondb"
)

// treeBlob is one file a tree holds: its git blob id and size.
type treeBlob struct {
	blob string
	size int64
}

// lsTree lists the blob each of paths has in commit, with one `git
// ls-tree` taking the paths literally; a path the commit does not track
// as a file is missing from the map, and so is every path when git cannot
// say in time.
func (rec *recorder) lsTree(wt, commit string, paths []string) map[string]treeBlob {
	if len(paths) == 0 {
		return nil
	}
	ctx, cancel := rec.gitCtx()
	defer cancel()
	args := append([]string{
		"--no-optional-locks", "--literal-pathspecs", "ls-tree", "-l", "-z", commit, "--",
	}, paths...)
	out, ok := gitBytes(ctx, wt, moondb.MaxStatusBytes, args...)
	if !ok {
		return nil
	}
	return parseLsTree(out)
}

// parseLsTree reads `git ls-tree -l -z` output: per entry "<mode> <type>
// <object> <size>\t<path>", NUL-terminated, the size padded with spaces.
// A submodule, a directory, or an entry of a shape it does not know is
// skipped.
func parseLsTree(out []byte) map[string]treeBlob {
	tree := map[string]treeBlob{}
	for _, entry := range strings.Split(string(out), "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || path == "" || len(fields) != 4 || fields[1] != "blob" {
			continue
		}
		size, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			continue
		}
		tree[path] = treeBlob{blob: fields[2], size: size}
	}
	return tree
}

// catBlobs reads the blobs oids names with one `git cat-file --batch` in
// wt, until ctx ends, and returns each one it read whole whose content
// matches its id.
func catBlobs(ctx context.Context, wt string, oids []string) map[string][]byte {
	if len(oids) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "--no-optional-locks", "cat-file", "--batch")
	cmd.Dir = wt
	cmd.WaitDelay = pipeGrace
	cmd.Stdin = strings.NewReader(strings.Join(oids, "\n") + "\n")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil
	}
	if err := cmd.Start(); err != nil {
		return nil
	}
	blobs := readBatch(bufio.NewReader(out), len(oids))
	// Done reading: git is finished, or stuck writing an answer cut short.
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return blobs
}

// readBatch reads up to n `git cat-file --batch` answers: per object a
// "<oid> <type> <size>" line, the content and a newline, or "<oid>
// missing" alone. It stops at the first answer it cannot read whole, and
// at one over the chunk cap.
func readBatch(r *bufio.Reader, n int) map[string][]byte {
	blobs := map[string][]byte{}
	for range n {
		header, err := r.ReadString('\n')
		if err != nil {
			break
		}
		fields := strings.Fields(header)
		if len(fields) != 3 {
			continue // missing
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 || size > moondb.MaxChunkBytes {
			break
		}
		data := make([]byte, size+1) // the content and its newline
		if _, err := io.ReadFull(r, data); err != nil {
			break
		}
		data = data[:size]
		if fields[1] == "blob" && blobID(data) == fields[0] {
			blobs[fields[0]] = data
		}
	}
	return blobs
}
