package rules

import (
	"path/filepath"
	"sync"
	"testing"
)

// TestAllGetConcurrent exercises the sync.Once-guarded lazy init under
// heavy concurrent access. If the load path were unsynchronized,
// go test -race would flag a write race on loadedRules / loadedByID.
func TestAllGetConcurrent(t *testing.T) {
	const goroutines = 32
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				rs := All()
				if len(rs) == 0 {
					t.Errorf("All() returned empty")
					return
				}
				if _, ok := Get(rs[0].ID); !ok {
					t.Errorf("Get(%q) failed", rs[0].ID)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestEnsurePackConcurrent exercises the per-destDir sync.Once in
// EnsurePack. Two concurrent callers to the same dest must serialise
// without racing on mkdir/write.
func TestEnsurePackConcurrent(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "vale")

	const goroutines = 16
	errs := make(chan error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			errs <- EnsurePack(dest)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("EnsurePack: %v", err)
		}
	}
}
