// Package envdefault reads a setting from the environment with a compiled
// fallback. It is the shared form of the idiom every component here uses to
// get flag > env > default precedence out of cobra: the environment read
// happens inside the function that computes the flag's default, so a flag
// the user passed still wins and no second resolution pass is needed.
//
// An unset or empty variable falls back silently. A variable that is set to
// something the type cannot hold warns once on stderr and then falls back,
// because a typo in a port number should not start the service on the wrong
// port in silence, and should not stop it from starting either.
package envdefault

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

// String returns the value of the environment variable name, or fallback
// when it is unset or empty.
func String(name, fallback string) string {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}
	return fallback
}

// Int returns the integer value of the environment variable name, or
// fallback when it is unset, empty, or does not parse. A set value that
// does not parse prints one warning line to stderr naming the variable, the
// value, and the default being used instead.
func Int(name string, fallback int) int {
	return intFrom(os.Stderr, name, fallback)
}

// intFrom is Int with the warning sink injected so tests can read it.
func intFrom(w io.Writer, name string, fallback int) int {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		fmt.Fprintf(w, "%s: unparseable value %q, using default %d\n", name, v, fallback)
		return fallback
	}
	return n
}

// Bool returns the boolean value of the environment variable name, or
// fallback when it is unset, empty, or does not parse. It accepts the forms
// strconv.ParseBool does (1, t, true, 0, f, false, in any case). A set value
// that does not parse warns once on stderr like Int.
func Bool(name string, fallback bool) bool {
	return boolFrom(os.Stderr, name, fallback)
}

// boolFrom is Bool with the warning sink injected so tests can read it.
func boolFrom(w io.Writer, name string, fallback bool) bool {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		fmt.Fprintf(w, "%s: unparseable value %q, using default %t\n", name, v, fallback)
		return fallback
	}
	return b
}
