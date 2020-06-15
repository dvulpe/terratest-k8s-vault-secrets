package tests

import (
	"fmt"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
)

func TestVaultClient(t *testing.T) {
	vaultResourcePath, err := filepath.Abs("../kubernetes/vault.yaml")
	require.NoError(t, err, "unexpected error when reading Vault manifest")
	options := k8s.NewKubectlOptions("", "")
	options.Namespace = "default"

	k8s.KubectlApply(t, options, vaultResourcePath)
	defer k8s.KubectlDelete(t, options, vaultResourcePath)

	k8s.WaitUntilServiceAvailable(t, options, "vault", 10, time.Minute)
	k8s.WaitUntilNumPodsCreated(t, options, metav1.ListOptions{LabelSelector: "app = vault"}, 1, 10, time.Minute)
	pods := k8s.ListPods(t, options, metav1.ListOptions{LabelSelector: "app = vault"})
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got: %v", len(pods))
	}
	k8s.WaitUntilPodAvailable(t, options, pods[0].Name, 60, 1*time.Second)

	tunnel := k8s.NewTunnel(options, k8s.ResourceTypeService, "vault", 0, 8200)
	tunnel.ForwardPort(t)
	v := NewClient(t, fmt.Sprintf("http://%s", tunnel.Endpoint()))
	v.Initialise(t)
	v.Configure(t)
	tunnel.Close()

	workloadResourcePath, err := filepath.Abs("../kubernetes/workload.yaml")
	require.NoError(t, err, "unexpected error when reading workload manifest")

	k8s.KubectlApply(t, options, workloadResourcePath)
	defer k8s.KubectlDelete(t, options, workloadResourcePath)
	k8s.WaitUntilNumPodsCreated(t, options, metav1.ListOptions{LabelSelector: "app = workload"}, 1, 100, time.Second)
	appPods := k8s.ListPods(t, options, metav1.ListOptions{LabelSelector: "app = workload"})
	if len(appPods) != 1 {
		t.Fatalf("expected 1 pod, got: %v", len(pods))
	}

	k8s.WaitUntilPodAvailable(t, options, appPods[0].Name, 50, time.Second)
}
