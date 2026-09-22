package k8sstore

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
)

// CRD is the Page custom resource definition, embedded so a test or a
// development cluster can install it without the deploy directory. The
// manifests under deploy/base carry a byte-identical copy.
//
//go:embed crd.yaml
var CRD []byte

const (
	group    = "present.thismoon.mad01.dev"
	version  = "v1alpha1"
	kind     = "Page"
	listKind = "PageList"
)

// GVR names the pages resource for the dynamic client.
var GVR = schema.GroupVersionResource{Group: group, Version: version, Resource: "pages"}

// crdGVR names custom resource definitions themselves, for InstallCRD.
var crdGVR = schema.GroupVersionResource{
	Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions",
}

// LabelEphemeral marks ephemeral pages so the sweeper lists only those.
const LabelEphemeral = group + "/ephemeral"

// InstallCRD creates the Page definition in the cluster, or updates its
// spec when it already exists, and waits until the API server reports it
// established. It is for tests and development clusters; a real deployment
// applies deploy/base, and its service account has no right to do this.
func InstallCRD(ctx context.Context, client dynamic.Interface) error {
	var want unstructured.Unstructured
	if err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(CRD), 4096).Decode(&want); err != nil {
		return fmt.Errorf("decode embedded crd: %w", err)
	}
	crds := client.Resource(crdGVR)
	name := want.GetName()
	_, err := crds.Create(ctx, &want, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := crds.Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("get crd %s: %w", name, getErr)
		}
		existing.Object["spec"] = want.Object["spec"]
		if _, err := crds.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update crd %s: %w", name, err)
		}
	} else if err != nil {
		return fmt.Errorf("create crd %s: %w", name, err)
	}
	return wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 30*time.Second, true,
		func(ctx context.Context) (bool, error) {
			got, err := crds.Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			conds, _, _ := unstructured.NestedSlice(got.Object, "status", "conditions")
			for _, c := range conds {
				m, _ := c.(map[string]any)
				if m["type"] == "Established" && m["status"] == "True" {
					return true, nil
				}
			}
			return false, nil
		})
}
