package k8sstore

import (
	_ "embed"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// CRD is the Page custom resource definition, embedded so a test or a
// development cluster can install it without the deploy directory. The
// manifests under deploy/base carry a byte-identical copy. Installing it is
// a test-only path (see installCRD in crd_install_test.go): a deployed
// instance applies deploy/base, and its service account may touch pages and
// nothing else.
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

// LabelEphemeral marks ephemeral pages so the sweeper lists only those.
const LabelEphemeral = group + "/ephemeral"
