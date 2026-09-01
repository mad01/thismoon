// Package sample is a fixture tree for the goscan extractor. It holds one
// declaration per extraction site so the table tests can pin them.
package sample

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrMissing is returned when the widget has no name.
var ErrMissing = errors.New("sample: widget name is required")

// command mimics a cobra command literal: the fields the extractor reads
// are Use, Short, Long, and Example.
var command = struct {
	Use     string
	Short   string
	Long    string
	Example string
}{
	Use:   "widget [name]",
	Short: "Build a widget from a name",
	Long: `Build a widget and write it to the store. The name is used as the
file name, so it has to be unique.`,
	Example: "  sample widget hinge",
}

// build makes a widget and reports the failures the caller can act on.
func build(name string) error {
	if name == "" {
		return ErrMissing
	}
	if len(name) > 64 {
		return fmt.Errorf("sample: widget name %q is longer than 64 characters", name)
	}
	return nil
}

func handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "sample: this endpoint only takes POST", http.StatusMethodNotAllowed)
		return
	}
	if err := build(r.URL.Query().Get("name")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
