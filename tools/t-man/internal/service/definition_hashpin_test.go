package service

import "testing"

// pinnedDefinition is a representative long-lived service: every field a
// service added before schedules existed would carry.
func pinnedDefinition() Definition {
	return Definition{
		Name:            "pinned",
		Command:         "/usr/bin/true",
		Args:            []string{"--port", "7423"},
		WorkingDir:      "/tmp",
		Environment:     map[string]string{"PATH": "/usr/bin"},
		RunAtLoad:       true,
		KeepAlive:       true,
		StandardOutPath: "/tmp/stdout.log",
		StandardErrPath: "/tmp/stderr.log",
	}
}

// TestDefinitionHash_UnscheduledIsPinned pins the literal hash of an
// unscheduled definition. A new Definition field must serialize to nothing
// when unset, or every service already on a machine reconciles (and
// bounces) on its next add; this test fails the moment that happens.
func TestDefinitionHash_UnscheduledIsPinned(t *testing.T) {
	const want = "94894e4d1e9fb768fc5b5546f176f449297b861c3dffe2200bd3b3eac71be166"
	def := pinnedDefinition()
	got, err := def.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if got != want {
		t.Errorf("unscheduled definition hash = %s, want %s (a new field changed the JSON of existing services)", got, want)
	}
}
