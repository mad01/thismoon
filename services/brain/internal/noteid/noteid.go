// Package noteid makes and checks note ids. A note id is a ULID: 26
// characters of Crockford base32 holding a 48-bit millisecond timestamp and
// 80 random bits, so ids sort by creation time and never repeat in
// practice. brain stores it in a note's id frontmatter field.
package noteid

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
	"time"
)

// Len is the length of a note id.
const Len = 26

// alphabet is Crockford's base32: no I, L, O or U.
const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// New returns a fresh note id stamped with now.
func New(now time.Time) string {
	var b [16]byte
	ms := uint64(now.UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	if _, err := rand.Read(b[6:]); err != nil {
		panic("noteid: crypto/rand failed: " + err.Error())
	}
	return encode(b)
}

// encode writes the 128 bits of b as 26 base32 digits, most significant
// first; the first digit carries the top 3 bits.
func encode(b [16]byte) string {
	hi := binary.BigEndian.Uint64(b[:8])
	lo := binary.BigEndian.Uint64(b[8:])
	var out [Len]byte
	for i := Len - 1; i >= 0; i-- {
		out[i] = alphabet[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(out[:])
}

// Valid reports whether s is a note id as New makes them.
func Valid(s string) bool {
	if len(s) != Len || s[0] > '7' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune(alphabet, rune(s[i])) {
			return false
		}
	}
	return true
}
