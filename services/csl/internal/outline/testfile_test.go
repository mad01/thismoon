package outline

import "testing"

func TestIsTestFile(t *testing.T) {
	yes := []string{
		"a_test.go", "pkg/b_test.go", "test_x.py", "x_test.py", "conftest.py",
		"app.test.ts", "app.spec.tsx", "lib/util.test.js", "FooTest.java", "FooTests.java",
		"test_run.sh", "run_test.bash", "__tests__/x.ts", "src/test/java/Foo.java",
		"tests/x.py", "pkg/testdata/in.go",
	}
	no := []string{
		"a.go", "testing.go", "contest.py", "app.ts", "latest.spec", "Testament.java",
		"kit/mcptest/mcptest.go", "protest/x.py", "test.md", "README.md",
	}
	for _, p := range yes {
		if !IsTestFile(p) {
			t.Errorf("IsTestFile(%q) = false, want true", p)
		}
	}
	for _, p := range no {
		if IsTestFile(p) {
			t.Errorf("IsTestFile(%q) = true, want false", p)
		}
	}
}
