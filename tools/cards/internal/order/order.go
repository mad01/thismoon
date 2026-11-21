// Package order names the end of a mode's screen that new cards go to, the
// --newest flag of both modes. List mode puts a card with a new id there;
// feed mode puts its newest entry there and follows it.
package order

import (
	"fmt"
	"slices"
	"strings"
)

// Newest is the end of the screen that new cards go to. The zero Newest is
// Bottom.
type Newest int

const (
	// Bottom puts new cards below the others.
	Bottom Newest = iota
	// Top puts new cards above the others.
	Top
)

// names holds each Newest's name, indexed by the constant.
var names = [...]string{Bottom: "bottom", Top: "top"}

// Names returns the names in order: bottom, top.
func Names() []string { return slices.Clone(names[:]) }

// String returns the name. A Newest outside the constants is Bottom.
func (n Newest) String() string {
	if n == Top {
		return names[Top]
	}
	return names[Bottom]
}

// MarshalText returns the name, so a Newest can back a flag.
func (n Newest) MarshalText() ([]byte, error) { return []byte(n.String()), nil }

// UnmarshalText sets n to the end named text, or returns an error naming
// the ends there are.
func (n *Newest) UnmarshalText(text []byte) error {
	i := slices.Index(names[:], string(text))
	if i < 0 {
		return fmt.Errorf("order: newest %q is not one of %s", text, strings.Join(Names(), ", "))
	}
	*n = Newest(i)
	return nil
}
