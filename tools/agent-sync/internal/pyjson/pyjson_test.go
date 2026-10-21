package pyjson

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// golden is testdata/python.json: every expected value in it was produced
// by Python's json module (testdata/golden.py), so these tests pin pyjson to
// Python's behavior, not to this package's reading of it.
type golden struct {
	Valid []struct {
		In, Compact, Indent, Indent0, Sorted string
	}
	Invalid []struct{ In, Err string }
	Equal   []struct {
		A, B  string
		Equal bool
	}
}

func loadGolden(t *testing.T) golden {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "python.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Valid) == 0 || len(g.Invalid) == 0 || len(g.Equal) == 0 {
		t.Fatal("testdata/python.json has an empty table")
	}
	return g
}

func mustLoads(t *testing.T, text string) any {
	t.Helper()
	v, err := Loads([]byte(text))
	if err != nil {
		t.Fatalf("Loads(%q) error: %v", text, err)
	}
	return v
}

func TestDumpsMatchesPython(t *testing.T) {
	for _, tc := range loadGolden(t).Valid {
		v := mustLoads(t, tc.In)
		checks := []struct{ name, got, want string }{
			{"json.dumps(v)", Compact(v), tc.Compact},
			{"json.dumps(v, indent=2)", Indent(v, 2), tc.Indent},
			{"json.dumps(v, indent=0)", Indent(v, 0), tc.Indent0},
			{"json.dumps(v, sort_keys=True)", Compact(SortKeys(v)), tc.Sorted},
		}
		for _, c := range checks {
			if c.got != c.want {
				t.Errorf("%s for %q:\n got %q\nwant %q", c.name, tc.In, c.got, c.want)
			}
		}
		// What Indent prints parses back to the same document.
		if again := mustLoads(t, Indent(v, 2)); Compact(again) != tc.Compact {
			t.Errorf("round trip of %q changed it to %q", tc.In, Compact(again))
		}
	}
}

func TestLoadsErrorsMatchPython(t *testing.T) {
	for _, tc := range loadGolden(t).Invalid {
		_, err := Loads([]byte(tc.In))
		var syntax *SyntaxError
		if !errors.As(err, &syntax) {
			t.Errorf("Loads(%q) error = %v, want a *SyntaxError", tc.In, err)
			continue
		}
		if err.Error() != tc.Err {
			t.Errorf("Loads(%q) error:\n got %q\nwant %q", tc.In, err.Error(), tc.Err)
		}
	}
}

func TestEqualMatchesPython(t *testing.T) {
	for _, tc := range loadGolden(t).Equal {
		a, b := mustLoads(t, tc.A), mustLoads(t, tc.B)
		if got := Equal(a, b); got != tc.Equal {
			t.Errorf("Equal(%s, %s) = %v, want %v (Python ==)", tc.A, tc.B, got, tc.Equal)
		}
		if got := Equal(b, a); got != tc.Equal {
			t.Errorf("Equal(%s, %s) = %v, want %v (Python ==)", tc.B, tc.A, got, tc.Equal)
		}
	}
}

func TestContainsUsesPythonEquality(t *testing.T) {
	list := mustLoads(t, `[1, "a", {"k": [true]}]`).([]any)
	tests := []struct {
		text string
		want bool
	}{
		{`1.0`, true},
		{`true`, true},
		{`"a"`, true},
		{`{"k": [1]}`, true},
		{`2`, false},
		{`"b"`, false},
	}
	for _, tc := range tests {
		if got := Contains(list, mustLoads(t, tc.text)); got != tc.want {
			t.Errorf("Contains(list, %s) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestObjectKeepsDictOrder(t *testing.T) {
	o := NewObject()
	o.Set("b", 1.0)
	o.Set("a", 2.0)
	o.Set("b", 3.0) // replaces in place, like d["b"] = 3
	if got := Compact(o); got != `{"b": 3.0, "a": 2.0}` {
		t.Fatalf("after Set: %s", got)
	}
	o.Delete("b")
	o.Delete("missing")
	o.Set("b", 4.0) // a deleted key comes back last
	if got := Compact(o); got != `{"a": 2.0, "b": 4.0}` {
		t.Fatalf("after Delete and Set: %s", got)
	}
	c := o.Clone()
	c.Set("c", nil)
	c.Delete("a")
	if got := Compact(o); got != `{"a": 2.0, "b": 4.0}` {
		t.Errorf("Clone shares state with the original: %s", got)
	}
}

func TestDeepCopySharesNothing(t *testing.T) {
	v := mustLoads(t, `{"a": {"b": [1, {"c": 2}]}}`)
	c := DeepCopy(v).(*Object)
	inner, _ := c.Get("a")
	inner.(*Object).Delete("b")
	if got := Compact(v); got != `{"a": {"b": [1, {"c": 2}]}}` {
		t.Errorf("DeepCopy shares state with the original: %s", got)
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("newlines are translated like read_text", func(t *testing.T) {
		path := write("crlf.json", []byte("{\r\n\"a\":\r\r}"))
		_, err := ReadFile(path)
		want := "Expecting value: line 4 column 1 (char 8)"
		if err == nil || err.Error() != want {
			t.Errorf("ReadFile error = %v, want %q", err, want)
		}
	})
	t.Run("invalid UTF-8 is an encoding error", func(t *testing.T) {
		path := write("latin1.json", []byte{'"', 'a', 0xe9, '"'})
		_, err := ReadFile(path)
		var enc *EncodingError
		if !errors.As(err, &enc) || enc.Pos != 2 || enc.Byte != 0xe9 {
			t.Errorf("ReadFile error = %v, want an *EncodingError at byte 2", err)
		}
	})
	t.Run("a missing file is fs.ErrNotExist", func(t *testing.T) {
		_, err := ReadFile(filepath.Join(dir, "absent.json"))
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ReadFile error = %v, want fs.ErrNotExist", err)
		}
	})
}
