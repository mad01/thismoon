package k8sstore

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/mad01/thismoon/services/present/internal/store"
)

func fakeFixture(t *testing.T) *fixture {
	t.Helper()
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{GVR: listKind})
	f := &fixture{now: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)}
	st, err := New(Config{Client: client, Namespace: "present-test", Now: func() time.Time { return f.now }})
	if err != nil {
		t.Fatal(err)
	}
	f.st = st
	return f
}

func TestFakeClientConformance(t *testing.T) {
	runConformance(t, fakeFixture)
}

func TestNewRequiresClientAndNamespace(t *testing.T) {
	if _, err := New(Config{Namespace: "x"}); err == nil {
		t.Error("New without a client must fail")
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{GVR: listKind})
	if _, err := New(Config{Client: client}); err == nil {
		t.Error("New without a namespace must fail")
	}
}

// TestUpdateRetriesOnConflict injects one resourceVersion conflict and
// expects the read-modify-write to run again and succeed.
func TestUpdateRetriesOnConflict(t *testing.T) {
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{GVR: listKind})
	st, err := New(Config{Client: client, Namespace: "present-test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	p, err := st.Create(ctx, store.Draft{Title: "T", Content: "<p>v1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	conflicts := 0
	client.PrependReactor("update", "pages", func(k8stesting.Action) (bool, runtime.Object, error) {
		if conflicts == 0 {
			conflicts++
			return true, nil, apierrors.NewConflict(GVR.GroupResource(), p.ID, nil)
		}
		return false, nil, nil
	})
	content := "<p>v2</p>"
	got, err := st.Update(ctx, p.ID, store.Patch{Content: &content})
	if err != nil {
		t.Fatalf("Update after one conflict: %v", err)
	}
	if got.Version != 2 || conflicts != 1 {
		t.Fatalf("version %d conflicts %d, want 2 and 1", got.Version, conflicts)
	}
}

func TestEphemeralLabelFollowsTheFlag(t *testing.T) {
	f := fakeFixture(t)
	ctx := context.Background()
	exp := f.now.Add(time.Hour)
	p, _ := f.st.Create(ctx, store.Draft{Title: "T", Content: "x", Ephemeral: true, ExpiresAt: &exp})
	_, u, err := f.st.get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if u.GetLabels()[LabelEphemeral] != "true" {
		t.Fatal("ephemeral page must carry the sweeper label")
	}
	off := false
	if _, err := f.st.Update(ctx, p.ID, store.Patch{Ephemeral: &off}); err != nil {
		t.Fatal(err)
	}
	_, u, _ = f.st.get(ctx, p.ID)
	if _, has := u.GetLabels()[LabelEphemeral]; has {
		t.Fatal("a page kept forever must drop the sweeper label")
	}
}
