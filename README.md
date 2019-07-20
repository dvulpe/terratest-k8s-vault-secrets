# vault-agent and consul-template sidecars for pods secrets 

see `tests/integration_test.go`

To run this make sure you have a kubernetes cluster and you can deploy to the `default` namespace (minikube works great)
Having go >= 1.12 installed, run:
```bash
cd tests && go test -v
```

The tests will:
* deploy a vault service 
* initialise and unseal Vault 
* mount a KV secrets engine and write a secret
* mount a PKI secrets engine and initialise a CA and a role
* enable kubernetes authentication and configure an authentication role for `app-sa` service account
* deploy a workload consisting of vault-agent and consul-template

Vault agent has to:
* read the service account token
* talk to vault and exchange it for a vault token 
* write the vault token to `/home/consul-template/.vault-token`
 
Consul-template will:
* read the vault token `/home/consul-template/.vault-token`
* read a password from vault and output it to a file
* request a certificate from vault and output it to a file

For manifests - see `kubernetes` folder and the actual test.
