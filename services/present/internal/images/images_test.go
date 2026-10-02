package images

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// pngBytes is a small PNG, the way a test would get a screenshot.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(1, 1, color.RGBA{R: 200, G: 90, B: 60, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 3)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gifBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gif.Encode(&buf, image.NewPaletted(image.Rect(0, 0, 4, 3), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const hash = "3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f3a7f"

func TestValidName(t *testing.T) {
	for _, ok := range []string{hash + ".png", hash + ".jpg", hash + ".jpeg", hash + ".gif", hash + ".webp"} {
		if !ValidName(ok) {
			t.Errorf("ValidName(%q) = false", ok)
		}
	}
	for _, bad := range []string{
		"", hash, hash + ".svg", hash + ".PNG", strings.ToUpper(hash) + ".png", hash[:63] + ".png",
		"../" + hash + ".png", hash + ".png/..", "a/" + hash + ".png", hash + ".png\n",
	} {
		if ValidName(bad) {
			t.Errorf("ValidName(%q) = true", bad)
		}
	}
}

func TestNameOfAndServed(t *testing.T) {
	name := hash + ".webp"
	if got, ok := NameOf(Served(name)); !ok || got != name {
		t.Errorf("NameOf(Served(%q)) = %q, %v", name, got, ok)
	}
	for _, bad := range []string{name, "/images/" + name, "/img/", "/img/x.png", "/img/../" + name, "https://x/img/" + name} {
		if _, ok := NameOf(bad); ok {
			t.Errorf("NameOf(%q) accepted", bad)
		}
	}
}

func TestReferenced(t *testing.T) {
	a, b := hash+".png", strings.ReplaceAll(hash, "3", "4")+".jpg"
	html := `<img src="/img/` + a + `" alt="a"> <img src="/img/` + b + `"> <img src="/img/` + a + `">` +
		` <img src="/img/nope.png"> {"src":"/img/` + a + `"}`
	if got := Referenced(html); !slices.Equal(got, []string{a, b}) {
		t.Errorf("Referenced = %v, want [%s %s]", got, a, b)
	}
	if got := Referenced("<p>no images</p>"); got != nil {
		t.Errorf("Referenced of plain HTML = %v", got)
	}
}

func TestContentType(t *testing.T) {
	cases := map[string]string{
		hash + ".png": "image/png", hash + ".jpg": "image/jpeg", hash + ".jpeg": "image/jpeg",
		hash + ".gif": "image/gif", hash + ".webp": "image/webp", hash + ".svg": "",
	}
	for name, want := range cases {
		if got := ContentType(name); got != want {
			t.Errorf("ContentType(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestSniff(t *testing.T) {
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 20)...)
	ok := []struct {
		name string
		data []byte
		ext  string
	}{
		{"png", pngBytes(t), "png"},
		{"jpeg", jpegBytes(t), "jpg"},
		{"gif", gifBytes(t), "gif"},
		{"webp", webp, "webp"},
	}
	for _, c := range ok {
		ext, err := Sniff(c.data)
		if err != nil || ext != c.ext {
			t.Errorf("%s: Sniff = %q, %v; want %q", c.name, ext, err, c.ext)
		}
	}
	bad := map[string][]byte{
		"text":  []byte("hello, world"),
		"svg":   []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"html":  []byte("<!DOCTYPE html><html><img></html>"),
		"empty": nil,
		"pdf":   []byte("%PDF-1.4 ..."),
	}
	for name, data := range bad {
		if ext, err := Sniff(data); err == nil {
			t.Errorf("%s: Sniff accepted as %q", name, ext)
		} else if !strings.Contains(err.Error(), "want png, jpeg, gif, or webp") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestNameIsContentHash(t *testing.T) {
	data := pngBytes(t)
	name := Name(data, "png")
	if !ValidName(name) || name != Name(slices.Clone(data), "png") {
		t.Errorf("Name = %q", name)
	}
	if Name(append(slices.Clone(data), 0), "png") == name {
		t.Error("different bytes got the same name")
	}
}

func TestStoreWriteOpenIdempotent(t *testing.T) {
	s := New(t.TempDir())
	data := pngBytes(t)
	name := Name(data, "png")
	if _, err := os.Stat(s.Dir()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("New created the directory: %v", err)
	}
	if err := s.Write(name, data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := s.Write(name, data); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	f, err := s.Open(name)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()
	got, _ := os.ReadFile(f.Name())
	if !bytes.Equal(got, data) {
		t.Error("stored bytes differ")
	}
	entries, _ := os.ReadDir(s.Dir())
	if len(entries) != 1 {
		t.Errorf("images dir holds %d entries, want 1 (no temp files left)", len(entries))
	}
	if info, err := os.Stat(f.Name()); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("stored file mode = %v, %v; want 0644 like the pages", info.Mode(), err)
	}
	if err := s.Write("../escape.png", data); err == nil {
		t.Error("Write accepted a traversal name")
	}
	for _, bad := range []string{"../" + name, "missing.png", strings.ReplaceAll(hash, "3", "5") + ".png"} {
		if _, err := s.Open(bad); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Open(%q) = %v, want ErrNotExist", bad, err)
		}
	}
}

func TestStoreSweep(t *testing.T) {
	s := New(t.TempDir())
	if removed, err := s.Sweep(nil, time.Now()); err != nil || removed != nil {
		t.Fatalf("Sweep of a store without a directory = %v, %v", removed, err)
	}
	keep := Name(pngBytes(t), "png")
	stale := Name(jpegBytes(t), "jpg")
	fresh := Name(gifBytes(t), "gif")
	for _, w := range []struct {
		name string
		data []byte
	}{{keep, pngBytes(t)}, {stale, jpegBytes(t)}, {fresh, gifBytes(t)}} {
		if err := s.Write(w.name, w.data); err != nil {
			t.Fatal(err)
		}
	}
	// A stray file that is not a stored name is never the sweep's to remove.
	stray := filepath.Join(s.Dir(), "notes.txt")
	if err := os.WriteFile(stray, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * sweepGrace)
	for _, name := range []string{keep, stale} {
		if err := os.Chtimes(filepath.Join(s.Dir(), name), old, old); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := s.Sweep(map[string]bool{keep: true}, time.Now())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if !slices.Equal(removed, []string{stale}) {
		t.Errorf("removed = %v, want [%s]", removed, stale)
	}
	for _, want := range []string{keep, fresh} {
		if _, err := os.Stat(filepath.Join(s.Dir(), want)); err != nil {
			t.Errorf("%s gone after sweep: %v", want, err)
		}
	}
	if _, err := os.Stat(stray); err != nil {
		t.Errorf("stray file removed: %v", err)
	}
}
