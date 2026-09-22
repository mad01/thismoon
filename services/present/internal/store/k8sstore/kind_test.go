package k8sstore

import (
	"context"
	"os"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// TestKindClusterConformance runs the conformance suite against a real
// cluster: the one the kubeconfig points at, which is kind locally and in
// CI. It is off unless PRESENT_KIND_TEST is set, so `go test ./...` never
// needs a cluster. It installs the Page CRD (never removes it) and works in
// a namespace of its own that it deletes afterwards.
func TestKindClusterConformance(t *testing.T) {
	if os.Getenv("PRESENT_KIND_TEST") == "" {
		t.Skip("set PRESENT_KIND_TEST=1 with a kubeconfig pointing at a kind cluster")
	}
	cfg, err := RESTConfig()
	if err != nil {
		t.Fatalf("RESTConfig: %v", err)
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("dynamic client: %v", err)
	}
	ctx := context.Background()
	if err := installCRD(ctx, client); err != nil {
		t.Fatalf("installCRD: %v", err)
	}
	ns := "present-test-" + store.NewID()
	nsGVR := schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	nsObj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": ns},
	}}
	if _, err := client.Resource(nsGVR).Create(ctx, nsObj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create namespace %s: %v", ns, err)
	}
	t.Cleanup(func() {
		_ = client.Resource(nsGVR).Delete(context.Background(), ns, metav1.DeleteOptions{})
	})

	runConformance(t, func(t *testing.T) *fixture {
		t.Helper()
		f := &fixture{now: time.Now().UTC().Truncate(time.Second)}
		st, err := New(
			Config{Client: client, Namespace: ns, Now: func() time.Time { return f.now }},
		)
		if err != nil {
			t.Fatal(err)
		}
		// Each subtest starts from an empty namespace so list counts hold.
		pages, err := st.List(context.Background())
		if err != nil {
			t.Fatalf("list before subtest: %v", err)
		}
		for _, p := range pages {
			_ = st.Delete(context.Background(), p.ID)
		}
		f.st = st
		return f
	})
}
