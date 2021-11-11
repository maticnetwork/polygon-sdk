package hashicorpvault

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/hashicorp/go-hclog"
	vault "github.com/hashicorp/vault/api"
)

// VaultSecretsManager is a SecretsManager that
// stores secrets on a Hashicorp Vault instance
type VaultSecretsManager struct {
	// Logger object
	logger hclog.Logger

	// Token used for Vault instance authentication
	token string

	// The Server URL of the Vault instance
	serverURL string

	// The name of the current node, used for prefixing names of secrets
	name string

	// The base path to store the secrets in the KV-2 Vault storage
	basePath string

	// The HTTP client used for interacting with the Vault server
	client *vault.Client
}

// SecretsManagerFactory implements the factory method
func SecretsManagerFactory(
	config *secrets.SecretsManagerParams,
) (secrets.SecretsManager, error) {
	// Set up the base object
	vaultManager := &VaultSecretsManager{
		logger: config.Logger.Named(string(secrets.HashicorpVault)),
	}

	// Grab the token from the config
	token, ok := config.Params[secrets.Token]
	if !ok {
		return nil, errors.New("no token specified for Vault secrets manager")
	}
	vaultManager.token = token.(string)

	// Grab the server URL from the config
	serverURL, ok := config.Params[secrets.Server]
	if !ok {
		return nil, errors.New("no server URL specified for Vault secrets manager")
	}
	vaultManager.serverURL = serverURL.(string)

	// Grab the node name from the config
	name, ok := config.Params[secrets.Name]
	if !ok {
		return nil, errors.New("no node name specified for Vault secrets manager")
	}
	vaultManager.name = name.(string)

	// Set the base path to store the secrets in the KV-2 Vault storage
	vaultManager.basePath = fmt.Sprintf("secret/data/%s", vaultManager.name)

	// Run the initial setup
	_ = vaultManager.Setup()

	return vaultManager, nil
}

// Setup sets up the Hashicorp Vault secrets manager
func (v *VaultSecretsManager) Setup() error {
	config := vault.DefaultConfig()

	// Set the server URL
	config.Address = v.serverURL
	client, err := vault.NewClient(config)
	if err != nil {
		return fmt.Errorf("unable to initialize Vault client: %v", err)
	}

	// Set the access token
	client.SetToken(v.token)

	v.client = client

	return nil
}

// constructSecretPath is a helper method for constructing a path to the secret
func (v *VaultSecretsManager) constructSecretPath(name string) string {
	return fmt.Sprintf("%s/%s", v.basePath, name)
}

// GetSecret fetches a secret from the Hashicorp Vault server
func (v *VaultSecretsManager) GetSecret(name string) (interface{}, error) {
	secret, err := v.client.Logical().Read(v.constructSecretPath(name))
	if err != nil {
		return nil, fmt.Errorf("unable to read secret from Vault, %v", err)
	}

	// KV-2 (versioned key-value storage) in Vault stores data in the following format:
	// {
	// "data": {
	// 		key: value
	// 	}
	// }
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf(
			"unable to assert type for secret from Vault, %T %#v",
			secret.Data["data"],
			secret.Data["data"],
		)
	}

	value, ok := data[name]
	if !ok {
		return nil, secrets.ErrSecretNotFound
	}

	return value, nil
}

// SetSecret saves a secret to the Hashicorp Vault server
func (v *VaultSecretsManager) SetSecret(name string, value interface{}) error {
	// Check if overwrite is possible
	_, err := v.GetSecret(name)
	if err == secrets.ErrSecretNotFound {
		v.logger.Warn(fmt.Sprintf("Overwriting secret: %s", name))
	}

	// Construct the data wrapper
	data := make(map[string]interface{})
	data[name] = value

	_, err = v.client.Logical().Write(v.constructSecretPath(name), map[string]interface{}{
		"data": data,
	})
	if err != nil {
		return fmt.Errorf("unable to store secret (%s), %v", name, err)
	}

	return nil
}

// HasSecret checks if the secret is present on the Hashicorp Vault server
func (v *VaultSecretsManager) HasSecret(name string) bool {
	_, err := v.GetSecret(name)

	return err == nil
}

// RemoveSecret removes a secret from the Hashicorp Vault server
func (v *VaultSecretsManager) RemoveSecret(name string) error {
	// Check if overwrite is possible
	_, err := v.GetSecret(name)
	if err != nil {
		return err
	}

	// Delete the secret from Vault storage
	_, err = v.client.Logical().Delete(v.constructSecretPath(name))
	if err != nil {
		return fmt.Errorf("unable to delete secret (%s), %v", name, err)
	}

	return nil
}
