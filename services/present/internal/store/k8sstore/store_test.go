package k8sstore

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// testNamespace is where the fake-client tests keep their pages.
const testNamespace = "present-test"

func newFakeClient() *fake.FakeDynamicClient {
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{GVR: listKind})
}

func fakeFixture(t *testing.T) *fixture {
	t.Helper()
	return fixtureOn(t, newFakeClient())
}

// fixtureOn builds a store over client in testNamespace with a clock the
// test controls. Its page cache is idle; startCache runs it.
func fixtureOn(t *testing.T, client dynamic.Interface) *fixture {
	t.Helper()
	f := &fixture{now: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)}
	st, err := New(
		Config{Client: client, Namespace: testNamespace, Now: func() time.Time { return f.now }},
	)
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
	p, _ := f.st.Create(
		ctx,
		store.Draft{Title: "T", Content: "x", Ephemeral: true, ExpiresAt: &exp},
	)
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

// TestCreateRetriesOnIDCollision injects one AlreadyExists and expects the
// store to mint a fresh id and try again — including when the caller
// supplied the id, which every shared write path does.
func TestCreateRetriesOnIDCollision(t *testing.T) {
	for _, tc := range []struct{ name, id string }{
		{"caller supplied the id", store.NewSharedID()},
		{"store minted the id", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
				map[schema.GroupVersionResource]string{GVR: listKind})
			st, err := New(Config{Client: client, Namespace: "present-test"})
			if err != nil {
				t.Fatal(err)
			}
			collisions := 0
			client.PrependReactor(
				"create",
				"pages",
				func(action k8stesting.Action) (bool, runtime.Object, error) {
					if collisions > 0 {
						return false, nil, nil
					}
					collisions++
					obj := action.(k8stesting.CreateAction).GetObject()
					name := obj.(*unstructured.Unstructured).GetName()
					return true, nil, apierrors.NewAlreadyExists(GVR.GroupResource(), name)
				},
			)
			ctx := context.Background()
			p, err := st.Create(ctx, store.Draft{ID: tc.id, Title: "T", Content: "<p>v1</p>"})
			if err != nil {
				t.Fatalf("Create after one collision: %v", err)
			}
			if collisions != 1 {
				t.Fatalf("collisions = %d, want 1", collisions)
			}
			if p.ID == tc.id || !store.ValidID(p.ID) {
				t.Fatalf("id = %q, want a fresh valid id (not %q)", p.ID, tc.id)
			}
			if got, err := st.Get(ctx, p.ID); err != nil || got.Title != "T" {
				t.Fatalf("page under the retried id = %+v, %v", got, err)
			}
		})
	}
}

// TestKeptPageCarriesEphemeralFalse pins the printer column: kubectl shows
// a blank EPHEMERAL cell when the field is absent, so a page kept until
// deleted must say so rather than say nothing.
func TestKeptPageCarriesEphemeralFalse(t *testing.T) {
	f := fakeFixture(t)
	ctx := context.Background()
	p, err := f.st.Create(ctx, store.Draft{Title: "Kept", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := f.st.pages().Get(ctx, p.ID, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := unstructured.NestedBool(u.Object, "spec", "ephemeral")
	if err != nil || !found {
		t.Fatalf("spec.ephemeral on a kept page: found %v, err %v", found, err)
	}
	if got {
		t.Errorf("spec.ephemeral = true on a kept page")
	}
}
