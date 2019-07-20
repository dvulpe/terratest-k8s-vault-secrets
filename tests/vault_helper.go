package tests

import (
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/hashicorp/go-retryablehttp"
	vault "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"testing"
	"time"
)

type Vault struct {
	v *vault.Client
}

func NewClient(t *testing.T, addr string) *Vault {
	v, err := vault.NewClient(&vault.Config{
		Address:    addr,
		Backoff:    retryablehttp.LinearJitterBackoff,
		MaxRetries: 10,
		Timeout:    5 * time.Second,
	})
	require.NoError(t, err, "failed building vault client")
	return &Vault{
		v: v,
	}
}

func (v *Vault) Initialise(t *testing.T) {
	log.Println("Initialising vault")
	init, err := v.v.Sys().Init(&vault.InitRequest{
		RecoveryShares:    1,
		RecoveryThreshold: 1,
		SecretShares:      1,
		SecretThreshold:   1,
	})
	require.NoError(t, err, "could not initialise vault: %v", err)
	log.Println("Initialised vault")
	log.Println("Unsealing vault")
	response, err := v.v.Sys().Unseal(init.Keys[0])
	require.NoError(t, err, "could not unseal vault: %v", err)
	if !response.Sealed {
		log.Println("Unsealed vault")
	}
	v.v.SetToken(init.RootToken)
}

func (v *Vault) Configure(t *testing.T) {
	v.configureAuth(t)
	v.configureSecretEngine(t)
	v.configurePki(t)
}

const policy = `
path "/secrets/kv/data/super-password" {
	capabilities = ["read"]
}

path "secrets/pki/issue/app-sa" {
	capabilities = ["update"]
}
`

func (v *Vault) configureAuth(t *testing.T) {
	auths, err := v.v.Sys().ListAuth()
	require.NoError(t, err, "could not list auth")
	if _, ok := auths["kubernetes/"]; !ok {
		log.Printf("Enabling k8s authentication")
		err = v.v.Sys().EnableAuthWithOptions("kubernetes", &vault.EnableAuthOptions{
			Type: "kubernetes",
		})
		require.NoError(t, err, "could not enable k8s auth")
	}
	clientset, _ := k8s.GetKubernetesClientE(t)
	account, err := clientset.CoreV1().ServiceAccounts("default").Get("vault-sa", metav1.GetOptions{})
	require.NoError(t, err, "could not read service account", err)
	tokenName := account.Secrets[0].Name
	secret, err := clientset.CoreV1().Secrets("default").Get(tokenName, metav1.GetOptions{})
	require.NoError(t, err, "could not retrieve secret")

	log.Println("Configuring vault auth")
	_, err = v.v.Logical().Write("auth/kubernetes/config", map[string]interface{}{
		"kubernetes_ca_cert": string(secret.Data["ca.crt"]),
		"kubernetes_host":    "https://kubernetes.default.svc.cluster.local",
		"token_reviewer_jwt": string(secret.Data["token"]),
	})
	require.NoError(t, err, "could not configure k8s auth")
	err = v.v.Sys().PutPolicy("app-sa", policy)
	require.NoError(t, err, "could not configure policy")
	_, err = v.v.Logical().Write("auth/kubernetes/role/app-sa", map[string]interface{}{
		"bound_service_account_names":      "app-sa",
		"bound_service_account_namespaces": "default",
		"policies":                         "default,app-sa",
		"ttl":                              "1h",
	})
	require.NoError(t, err, "could not write app-sa role")
}

func (v *Vault) configureSecretEngine(t *testing.T) {
	mounts, err := v.v.Sys().ListMounts()
	require.NoError(t, err, "could not list mounts")
	if _, ok := mounts["secrets/kv/"]; !ok {
		log.Println("enabling KV(2) engine")
		err := v.v.Sys().Mount("secrets/kv", &vault.MountInput{Type: "kv", Options: map[string]string{"version": "2"}})
		require.NoError(t, err, "could not enable KV mount")
	}
	log.Println("Writing secret to vault")
	_, err = v.v.Logical().Write("/secrets/kv/data/super-password", map[string]interface{}{
		"data": map[string]interface{}{
			"password": 42,
		},
	})
	require.NoError(t, err, "could not write password to vault")
}

func (v *Vault) configurePki(t *testing.T) {
	mounts, err := v.v.Sys().ListMounts()
	require.NoError(t, err, "could not list mounts")
	if _, ok := mounts["secrets/pki/"]; !ok {
		log.Println("Enabling PKI mount")
		err = v.v.Sys().Mount("secrets/pki", &vault.MountInput{Type: "pki"})
		require.NoError(t, err, "could not enable PKI mount")
	}
	_, err = v.v.Logical().Write("secrets/pki/root/generate/internal", map[string]interface{}{"common_name": "Internal CA"})
	require.NoError(t, err, "could not generate ROOT ca")
	_, err = v.v.Logical().Write("secrets/pki/roles/app-sa", map[string]interface{}{"max_ttl": "72h", "allowed_domains": "internal", "allow_subdomains": true})
	require.NoError(t, err, "could not create app-sa role")
}
