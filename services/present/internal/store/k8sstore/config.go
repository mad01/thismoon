package k8sstore

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// serviceAccountNamespaceFile is where a pod finds its own namespace.
const serviceAccountNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// RESTConfig returns the in-cluster configuration when running in a pod,
// else the default kubeconfig (KUBECONFIG or ~/.kube/config), which is how a
// developer points the store at a kind cluster.
func RESTConfig() (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return tune(cfg), nil
	}
	if !errors.Is(err, rest.ErrNotInCluster) {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	cfg, err = defaultClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig: %w", err)
	}
	return tune(cfg), nil
}

// tune lifts the client-go defaults (5 QPS, burst 10), which are sized for
// controllers, not for a request-serving process.
func tune(cfg *rest.Config) *rest.Config {
	cfg.QPS = 50
	cfg.Burst = 100
	return cfg
}

// ResolveNamespace picks the namespace pages live in: the flag when set,
// else the pod's service-account namespace, else the kubeconfig context's,
// else "default".
func ResolveNamespace(flag string) string {
	if flag != "" {
		return flag
	}
	if b, err := os.ReadFile(serviceAccountNamespaceFile); err == nil {
		if ns := strings.TrimSpace(string(b)); ns != "" {
			return ns
		}
	}
	if ns, _, err := defaultClientConfig().Namespace(); err == nil && ns != "" {
		return ns
	}
	return "default"
}

func defaultClientConfig() clientcmd.ClientConfig {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
}
