// Package pyjson reads and writes JSON the way Python's json module does, so
// agent-sync can replace the Python sync scripts byte for byte.
//
// A decoded document is a tree of plain Go values: nil (null), bool,
// *big.Int (an integer, any size), float64 (a number with a fraction or an
// exponent), string, []any (an array) and *Object (an object that keeps its
// keys in insertion order, like a Python dict). Equal compares two trees with
// Python's == rules, Loads parses like json.loads, and Compact and Indent
// print like json.dumps.
//
// Strings hold UTF-8, except that a lone UTF-16 surrogate from a \uXXXX
// escape is kept as its three-byte WTF-8 form, the way a Python str keeps
// it, so it survives a parse and print round trip.
package pyjson

import (
	"maps"
	"math"
	"math/big"
	"slices"
)

// Object is a JSON object that keeps its keys in insertion order. Setting a
// key that is already present replaces the value in place, and deleting one
// closes the gap: the same rules a Python dict follows. The zero value is
// not ready to use; call NewObject.
type Object struct {
	keys []string
	vals map[string]any
}

// NewObject returns an empty Object.
func NewObject() *Object {
	return &Object{vals: map[string]any{}}
}

// Len returns the number of keys.
func (o *Object) Len() int { return len(o.keys) }

// Keys returns the keys in insertion order. The slice is a copy.
func (o *Object) Keys() []string {
	return append([]string(nil), o.keys...)
}

// Get returns the value at key and whether the key is present.
func (o *Object) Get(key string) (any, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// Has reports whether key is present.
func (o *Object) Has(key string) bool {
	_, ok := o.vals[key]
	return ok
}

// Set stores v at key. A new key goes last; an existing key keeps its
// position.
func (o *Object) Set(key string, v any) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = v
}

// Delete removes key. A missing key is a no-op, like dict.pop(key, None).
func (o *Object) Delete(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i:i], o.keys[i+1:]...)
			return
		}
	}
}

// Clone returns a shallow copy: a new key order and map over the same
// values, like dict(o).
func (o *Object) Clone() *Object {
	return &Object{keys: o.Keys(), vals: maps.Clone(o.vals)}
}

// DeepCopy returns a copy of v that shares no Object or array with it, like
// copy.deepcopy. Scalars are returned as they are; *big.Int values are
// never mutated, so sharing them is safe.
func DeepCopy(v any) any {
	switch t := v.(type) {
	case *Object:
		c := &Object{keys: t.Keys(), vals: make(map[string]any, len(t.vals))}
		for k, val := range t.vals {
			c.vals[k] = DeepCopy(val)
		}
		return c
	case []any:
		c := make([]any, len(t))
		for i, val := range t {
			c[i] = DeepCopy(val)
		}
		return c
	default:
		return v
	}
}

// SortKeys returns a deep copy of v with every object's keys in sorted
// order, so Compact(SortKeys(v)) prints what json.dumps(v, sort_keys=True)
// prints. Byte order of the UTF-8 (or WTF-8) keys is code point order, which
// is the order Python sorts str keys in.
func SortKeys(v any) any {
	switch t := v.(type) {
	case *Object:
		keys := t.Keys()
		slices.Sort(keys)
		c := NewObject()
		for _, k := range keys {
			val, _ := t.Get(k)
			c.Set(k, SortKeys(val))
		}
		return c
	case []any:
		c := make([]any, len(t))
		for i, val := range t {
			c[i] = SortKeys(val)
		}
		return c
	default:
		return v
	}
}

// Equal reports whether a and b are equal under Python's == for the values
// json.loads produces. Objects compare by key set and values, ignoring key
// order; arrays compare element by element. Numbers compare by value across
// kinds, and a bool counts as the integer 0 or 1, so true equals 1 and 1.0
// the way it does in Python.
//
// A NaN equals a NaN. That matches Python inside a list or dict, whose ==
// takes an identity shortcut, because json.loads returns one shared NaN
// object for every NaN it parses. It does not match two bare floats: there
// Python's == and != have no shortcut and NaN != NaN. sync_settings.py
// compared values under an object key that way, so it reported a NaN the
// template sets as drift on every run; agent-sync reports it in sync.
func Equal(a, b any) bool {
	if na, ok := number(a); ok {
		nb, ok := number(b)
		return ok && numbersEqual(na, nb)
	}
	switch x := a.(type) {
	case nil:
		return b == nil
	case string:
		y, ok := b.(string)
		return ok && x == y
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !Equal(x[i], y[i]) {
				return false
			}
		}
		return true
	case *Object:
		y, ok := b.(*Object)
		if !ok || x.Len() != y.Len() {
			return false
		}
		for _, k := range x.keys {
			yv, ok := y.vals[k]
			if !ok || !Equal(x.vals[k], yv) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// Contains reports whether list holds an element Equal to v, like Python's
// `v in list`.
func Contains(list []any, v any) bool {
	return slices.ContainsFunc(list, func(item any) bool { return Equal(item, v) })
}

// num is a number in comparable form: exactly one of i and f is meaningful.
type num struct {
	i     *big.Int
	f     float64
	isInt bool
}

// number converts the numeric kinds (bool, *big.Int, float64) for
// comparison and reports false for every other kind.
func number(v any) (num, bool) {
	switch t := v.(type) {
	case bool:
		if t {
			return num{i: big.NewInt(1), isInt: true}, true
		}
		return num{i: big.NewInt(0), isInt: true}, true
	case *big.Int:
		return num{i: t, isInt: true}, true
	case float64:
		return num{f: t}, true
	default:
		return num{}, false
	}
}

// numbersEqual compares exactly, the way Python compares an int with a
// float: no rounding of the integer to the nearest float. Two NaNs are
// equal, as Python's container == finds them; see Equal for where Python's
// bare float comparison differs.
func numbersEqual(a, b num) bool {
	switch {
	case a.isInt && b.isInt:
		return a.i.Cmp(b.i) == 0
	case !a.isInt && !b.isInt:
		return a.f == b.f || (math.IsNaN(a.f) && math.IsNaN(b.f))
	case a.isInt:
		return intEqualsFloat(a.i, b.f)
	default:
		return intEqualsFloat(b.i, a.f)
	}
}

func intEqualsFloat(i *big.Int, f float64) bool {
	if math.IsInf(f, 0) || math.IsNaN(f) || f != math.Trunc(f) {
		return false
	}
	fi, _ := new(big.Float).SetFloat64(f).Int(nil)
	return fi.Cmp(i) == 0
}
