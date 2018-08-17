package v1alpha1

type ProviderSpec struct {
	Vault *VaultSpec `json:"vault,omitempty"`
}

// VaultSpec contains the information that necessary to talk with vault
type VaultSpec struct {
	// Specifies the address of the vault server, e.g:'http://127.0.0.1:8200'
	Address string `json:"address"`

	// Name of the secret containing the vault token
	// access permission:
	// secret data:
	//	- token:<value>
	TokenSecret string `json:"tokenSecret"`

	// To skip tls verification for vault server
	SkipTLSVerification bool `json:"skipTLSVerification,omitempty"`

	// Name of the secret containing the ca cert to verify vault server
	// secret data:
	//	- ca.crt:<value>
	ServerCASecret string `json:"server_ca_secret,omitempty"`

	// Name of the secret containing the client.srt and client.key
	// secret data:
	//	- client.crt: <value>
	//	- client.srt: <value>
	ClientTLSSecret string `json:"clientTLSSecret,omitempty"`
}

// Store specifies where to store credentials
type Store struct {
	// Specifies the name of the secret
	Secret string `json:"secret"`
}

// LeaseData contains lease info
type LeaseData struct {
	// lease id
	ID string `json:"id,omitempty"`

	// lease duration in seconds
	Duration int64 `json:"duration,omitempty"`

	// lease renew deadline in Unix time
	RenewDeadline int64 `json:"renewDeadline,omitempty"`
}
