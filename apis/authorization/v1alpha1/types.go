package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceKindPostgresRole = "PostgresRole"
	ResourcePostgresRole     = "postgresrole"
	ResourcePostgresRoles    = "postgresroles"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PostgresRole
type PostgresRole struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PostgresRoleSpec   `json:"spec,omitempty"`
	Status            PostgresRoleStatus `json:"status,omitempty"`
}

// PostgresRoleSpec contains connection information, postgres role info etc
type PostgresRoleSpec struct {
	Provider *ProviderSpec `json:"provider"`
	Database *DatabaseSpec `json:"database,omitempty"`

	// links:
	// 	- https://www.vaultproject.io/api/secret/databases/index.html
	//	- https://www.vaultproject.io/api/secret/databases/postgresql.html

	// The name of the database connection to use for this role.
	DBName string `json:"dbName"`

	// Specifies the TTL for the leases associated with this role.
	// Accepts time suffixed strings ("1h") or an integer number of seconds.
	// Defaults to system/engine default TTL time
	DefaultTTL string `json:"defaultTTL,omitempty"`

	// Specifies the maximum TTL for the leases associated with this role.
	// Accepts time suffixed strings ("1h") or an integer number of seconds.
	// Defaults to system/engine default TTL time.
	MaxTTL string `json:"maxTTL,omitempty"`

	// https://www.vaultproject.io/api/secret/databases/postgresql.html#creation_statements
	// Specifies the database statements executed to create and configure a user.
	CreationStatements []string `json:"creationStatements"`

	// https://www.vaultproject.io/api/secret/databases/postgresql.html#revocation_statements
	// Specifies the database statements to be executed to revoke a user.
	RevocationStatements []string `json:"revocationStatements,omitempty"`

	// https://www.vaultproject.io/api/secret/databases/postgresql.html#rollback_statements
	// Specifies the database statements to be executed rollback a create operation in the event of an error.
	RollbackStatements []string `json:"rollbackStatements,omitempty"`

	// https://www.vaultproject.io/api/secret/databases/postgresql.html#renew_statements
	// Specifies the database statements to be executed to renew a user.
	RenewStatements []string `json:"renewStatements,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PostgresRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of PostgresRole objects
	Items []PostgresRole `json:"items,omitempty"`
}

type PostgresRoleStatus struct {
	// observedGeneration is the most recent generation observed for this PostgresROle. It corresponds to the
	// PostgresROle's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

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

// vault doc:
// 	- https://www.vaultproject.io/api/secret/databases/postgresql.html
// 	- https://www.vaultproject.io/api/secret/databases/index.html
// DatabaseSpec contains connection url, connection settings, credential information
type DatabaseSpec struct {
	// Specifies the name for this database connection
	Name string `json:"name"`

	// Specifies the PostgreSQL DSN. This field can be templated and supports
	// passing the username and password parameters in the following format {{field_name}}.
	// A templated connection URL is required when using root credential rotation.
	// e.g. postgresql://{{username}}:{{password}}@localhost:5432/postgres?sslmode=disable
	ConnectionUrl string `json:"connectionUrl"`

	// Name of secret containing the username and password
	// secret data:
	//	- username: <value>
	//	- password: <value>
	CredentialSecret string `json:"credentialSecret"`

	// List of the roles allowed to use this connection.
	// Defaults to empty (no roles), if contains a "*" any role can use this connection.
	AllowedRoles string `json:"allowedRoles,omitempty"`

	// Specifies the maximum number of open connections to the database.
	MaxOpenConnections int `json:"maxOpenConnections,omitempty"`

	// Specifies the maximum number of idle connections to the database.
	// A zero uses the value of max_open_connections and a negative value disables idle connections.
	// If larger than max_open_connections it will be reduced to be equal.
	MaxIdleConnections int `json:"maxIdleConnections,omitempty"`

	// Specifies the maximum amount of time a connection may be reused.
	// If <= 0s connections are reused forever.
	MaxConnectionLifetime string `json:"maxConnectionLifetime,omitempty"`
}
