package storetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math/rand/v2"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// chunksRoundTrip: Put returns the lowercase hex SHA-256 of the bytes and
// is idempotent, Has and Get find what Put stored, and an unknown key is
// absent to Has and an error to Get.
func chunksRoundTrip(t *testing.T, st store.Store) {
	ctx := context.Background()
	rng := rand.New(rand.NewPCG(1, 2))
	big := make([]byte, 256<<10)
	for i := range big {
		big[i] = byte(rng.Uint32())
	}
	cases := []struct {
		name string
		data []byte
	}{
		{"text", []byte("package main\n\nfunc main() {}\n")},
		{"empty", []byte{}},
		{"binary", big},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sum := sha256.Sum256(c.data)
			want := hex.EncodeToString(sum[:])
			for range 2 {
				sha, err := st.Chunks().Put(ctx, c.data)
				if err != nil || sha != want {
					t.Fatalf("Put = %q, %v; want %q", sha, err, want)
				}
			}
			if ok, err := st.Chunks().Has(ctx, want); !ok || err != nil {
				t.Errorf("Has(%s) = %v, %v; want true", want, ok, err)
			}
			got, err := st.Chunks().Get(ctx, want)
			if err != nil || !bytes.Equal(got, c.data) {
				t.Errorf(
					"Get(%s) = %d bytes, %v; want the %d bytes put",
					want,
					len(got),
					err,
					len(c.data),
				)
			}
		})
	}

	unknown := hex.EncodeToString(make([]byte, sha256.Size))
	if ok, err := st.Chunks().Has(ctx, unknown); ok || err != nil {
		t.Errorf("Has(unknown) = %v, %v; want false, nil", ok, err)
	}
	if _, err := st.Chunks().Get(ctx, unknown); err == nil {
		t.Error("Get(unknown) succeeded, want an error")
	}
}

// ping: an open store answers.
func ping(t *testing.T, st store.Store) {
	if err := st.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}
