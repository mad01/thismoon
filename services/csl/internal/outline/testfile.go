package outline

import (
	"path/filepath"
	"strings"
)

// testDirs are directory names whose whole subtree counts as tests.
var testDirs = map[string]bool{
	"__tests__": true,
	"test":      true,
	"tests":     true,
	"testdata":  true,
}

// IsTestFile reports whether rel is a test file by the naming conventions
// of the languages the extractor covers: Go _test.go; Python test_*.py,
// *_test.py and conftest.py; TypeScript and JavaScript *.test.* and
// *.spec.*; Java *Test.java and *Tests.java; shell test_*.sh and *_test.sh;
// and anything under a __tests__, test, tests, or testdata directory.
func IsTestFile(rel string) bool {
	for _, dir := range strings.Split(filepath.Dir(rel), string(filepath.Separator)) {
		if testDirs[dir] {
			return true
		}
	}
	return isTestName(filepath.Base(rel))
}

// isTestName applies the per-language file-name conventions to a base name.
func isTestName(base string) bool {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	switch ext {
	case ".go":
		return strings.HasSuffix(stem, "_test")
	case ".py":
		return strings.HasPrefix(stem, "test_") || strings.HasSuffix(stem, "_test") ||
			base == "conftest.py"
	case ".ts", ".tsx", ".js", ".jsx":
		second := filepath.Ext(stem)
		return second == ".test" || second == ".spec"
	case ".java":
		return strings.HasSuffix(stem, "Test") || strings.HasSuffix(stem, "Tests")
	case ".sh", ".bash":
		return strings.HasPrefix(stem, "test_") || strings.HasSuffix(stem, "_test")
	}
	return false
}
