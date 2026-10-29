package export

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Each face in the cached fonts.css follows a comment naming its Unicode
// subset, and names its file under /assets/fonts/.
var (
	reFontFace = regexp.MustCompile(`(?s)/\* ([a-z0-9-]+) \*/\s*(@font-face \{.*?\})`)
	reFontURL  = regexp.MustCompile(`url\(/assets/fonts/([A-Za-z0-9_-]+\.woff2)\)`)
)

// latinFonts returns the latin faces of the cached font stylesheet with each
// woff2 file inlined as a data URI. The families are not on the npm CDN in
// the bytes Google Fonts serves, so the export carries them itself, latin
// only to keep the file small; other scripts fall back to system fonts. A
// cache without the stylesheet yields no faces and a note, and the page
// reads in system fonts.
func latinFonts(assets string) (string, string, error) {
	css, err := os.ReadFile(filepath.Join(assets, "css", "fonts.css"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", "fonts.css is not in the asset cache, so the page uses system fonts", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("export: read fonts: %w", err)
	}
	var b strings.Builder
	for _, m := range reFontFace.FindAllStringSubmatch(string(css), -1) {
		if m[1] != "latin" {
			continue
		}
		face, err := inlineFontFiles(assets, m[2])
		if err != nil {
			return "", "", err
		}
		b.WriteString(face)
		b.WriteByte('\n')
	}
	return b.String(), "", nil
}

// inlineFontFiles swaps every cached font URL in one @font-face rule for a
// data URI of the file.
func inlineFontFiles(assets, face string) (string, error) {
	var readErr error
	out := reFontURL.ReplaceAllStringFunc(face, func(ref string) string {
		name := reFontURL.FindStringSubmatch(ref)[1]
		data, err := os.ReadFile(filepath.Join(assets, "fonts", name))
		if err != nil {
			readErr = fmt.Errorf("export: font %s: %w", name, err)
			return ref
		}
		return "url(" + dataURI("font/woff2", data) + ")"
	})
	return out, readErr
}
