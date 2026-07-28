package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// logf reports non-fatal store conditions (torn lines); tests swap it to
// capture warnings.
var logf = log.Printf

// scanJSONL reads one append-only log and hands each record to visit in file
// order. A missing file is not an error — it is an empty log.
//
// An unparseable line is fatal with its file and line number, except a torn
// final line without its newline: that is the crash artifact of an interrupted
// append, so it is dropped and truncated away. A final line that parsed but
// lost its newline gets one appended, so the next append cannot merge into it.
// Repair is safe here because scanJSONL only runs at startup, before the serve
// process starts writing.
func scanJSONL[T any](path, name string, visit func(T)) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()

	r := bufio.NewReader(f)
	var offset int64
	for lineNo := 1; ; lineNo++ {
		line, readErr := r.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("read %s: %w", name, readErr)
		}
		complete := strings.HasSuffix(line, "\n")
		if text := strings.TrimSpace(line); text != "" {
			var rec T
			switch uerr := json.Unmarshal([]byte(text), &rec); {
			case uerr == nil:
				visit(rec)
				if !complete {
					if aerr := appendNewline(path); aerr != nil {
						return fmt.Errorf("repair %s: %w", name, aerr)
					}
				}
			case !complete:
				logf("wire: dropping torn final line %s:%d (interrupted append)", name, lineNo)
				if terr := os.Truncate(path, offset); terr != nil {
					return fmt.Errorf("repair %s: %w", name, terr)
				}
			default:
				return fmt.Errorf("parse %s:%d: %w", name, lineNo, uerr)
			}
		}
		offset += int64(len(line))
		if readErr != nil { // io.EOF after the final line
			return nil
		}
	}
}

// appendNewline terminates a final line that parsed but lost its newline.
func appendNewline(path string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write([]byte("\n"))
	return err
}

// appendJSONL persists one record as one line. The append is the atomic unit,
// so there is no temp-file-and-rename dance.
func appendJSONL(path, name string, rec any) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("append %s: %w", name, err)
	}
	return nil
}
