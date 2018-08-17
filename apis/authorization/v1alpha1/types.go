package v1alpha1

import (
	"k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceKindPostgresRole = "PostgresRole"
	ResourcePostgresRole     = "postgresrole"
	ResourcePostgresRoles    = "postgresroles"

	ResourceKindMysqlRole = "MysqlRole"
	ResourceMysqlRole     = "mysqlrole"
	ResourceMysqlRoles    = "mysqlroles"

	ResourceKindMongodbRole = "MongodbRole"
	ResourceMongodbRole     = "mongodbrole"
	ResourceMongodbRoles    = "mongodbroles"

	ResourceKindPostgresRoleBinding = "PostgresRoleBinding"
	ResourcePostgresRoleBinding     = "postgresrolebinding"
	ResourcePostgresRoleBindings    = "postgresrolebindings"

	ResourceKindMysqlRoleBinding = "MysqlRoleBinding"
	ResourceMysqlRoleBinding     = "mysqlrolebinding"
	ResourceMysqlRoleBindings    = "mysqlrolebindings"

	ResourceKindMongodbRoleBinding = "MongodbRoleBinding"
	ResourceMongodbRoleBinding     = "mongodbrolebinding"
	ResourceMongodbRoleBindings    = "mongodbrolebindings"
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
	Provider *ProviderSpec              `json:"provider"`
	Database *DatabaseConfigForPostgres `json:"database,omitempty"`

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

type PostgresRolePhase string

type PostgresRoleStatus struct {
	// observedGeneration is the most recent generation observed for this PostgresROle. It corresponds to the
	// PostgresROle's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Specifies the phase of the PostgresRole
	Phase PostgresRolePhase `json:"phase,omitempty"`

	// Represents the latest available observations of a PostgresRoleBinding current state.
	Conditions []PostgresRoleCondition `json:"conditions,omitempty"`
}

// PostgresRoleCondition describes the state of a PostgresRole at a certain point.
type PostgresRoleCondition struct {
	// Type of PostgresRole condition.
	Type string `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status,omitempty"`

	// The reason for the condition's.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
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

// https://www.vaultproject.io/api/secret/databases/index.html
// https://www.vaultproject.io/api/secret/databases/postgresql.html#configure-connection
// DatabaseConfigForPostgres contains database connection config
type DatabaseConfigForPostgres struct {
	// Specifies the name for this database connection
	Name string `json:"name"`

	// Specifies the name of the plugin to use for this connection.
	// Default plugin:
	//	- for postgres: postgresql-database-plugin
	//  - for mysql: mysql-database-plugin
	PluginName string `json:"pluginName,omitempty"`

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

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PostgresRoleBinding binds postgres credential to user
type PostgresRoleBinding struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PostgresRoleBindingSpec   `json:"spec,omitempty"`
	Status            PostgresRoleBindingStatus `json:"status,omitempty"`
}

type PostgresRoleBindingSpec struct {
	// Specifies the name of the PostgresRole
	RoleRef string `json:"roleRef"`

	Subjects []rbacv1.Subject `json:"subjects"`

	Store Store `json:"store"`
}

// Store specifies where to store credentials
type Store struct {
	// Specifies the name of the secret
	Secret string `json:"secret"`
}

type PostgresRoleBindingPhase string

type PostgresRoleBindingStatus struct {
	// observedGeneration is the most recent generation observed for this PostgresRoleBinding. It corresponds to the
	// PostgresRoleBinding's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// contains lease info of the credentials
	Lease LeaseData `json:"lease,omitempty"`

	// Specifies the phase of the postgres role binding
	Phase PostgresRoleBindingPhase `json:"phase,omitempty"`

	// Represents the latest available observations of a PostgresRoleBinding current state.
	Conditions []PostgresRoleBindingCondition `json:"conditions,omitempty"`
}

// PostgresRoleBindingCondition describes the state of a PostgresRoleBinding at a certain point.
type PostgresRoleBindingCondition struct {
	// Type of PostgresRoleBinding condition.
	Type string `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status,omitempty"`

	// The reason for the condition's.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PostgresRoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of PostgresRoleBinding objects
	Items []PostgresRoleBinding `json:"items,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MysqlRole
type MysqlRole struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MysqlRoleSpec   `json:"spec,omitempty"`
	Status            MysqlRoleStatus `json:"status,omitempty"`
}

// MysqlRoleSpec contains connection information, mysql role info etc
type MysqlRoleSpec struct {
	Provider *ProviderSpec           `json:"provider"`
	Database *DatabaseConfigForMysql `json:"database,omitempty"`

	// links:
	// 	- https://www.vaultproject.io/api/secret/databases/index.html
	//	- https://www.vaultproject.io/api/secret/databases/mysql-maria.html

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

	// https://www.vaultproject.io/api/secret/databases/mysql-maria.html#creation_statements
	// Specifies the database statements executed to create and configure a user.
	CreationStatements []string `json:"creationStatements"`

	// https://www.vaultproject.io/api/secret/databases/mysql-maria.html#revocation_statements
	// Specifies the database statements to be executed to revoke a user.
	RevocationStatements []string `json:"revocationStatements,omitempty"`
}

// https://www.vaultproject.io/api/secret/databases/index.html
// https://www.vaultproject.io/api/secret/databases/mysql-maria.html#configure-connection
// DatabaseConfigForMysql contains database connection config
type DatabaseConfigForMysql struct {
	// Specifies the name for this database connection
	Name string `json:"name"`

	// Specifies the name of the plugin to use for this connection.
	// Default plugin:
	//	- for postgres: postgresql-database-plugin
	//  - for mysql: mysql-database-plugin
	PluginName string `json:"pluginName,omitempty"`

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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MysqlRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of MysqlRole objects
	Items []MysqlRole `json:"items,omitempty"`
}

type MysqlRolePhase string

type MysqlRoleStatus struct {
	Phase MysqlRolePhase `json:"phase,omitempty"`

	// observedGeneration is the most recent generation observed for this MysqlRole. It corresponds to the
	// MysqlRole's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Represents the latest available observations of a MysqlRole current state.
	Conditions []MysqlRoleCondition `json:"conditions,omitempty"`
}

// MysqlRoleCondition describes the state of a MysqlRole at a certain point.
type MysqlRoleCondition struct {
	// Type of MysqlRole condition.
	Type string `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status,omitempty"`

	// The reason for the condition's.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MysqlRoleBinding binds mysql credential to user
type MysqlRoleBinding struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MysqlRoleBindingSpec   `json:"spec,omitempty"`
	Status            MysqlRoleBindingStatus `json:"status,omitempty"`
}

type MysqlRoleBindingSpec struct {
	// Specifies the name of the MysqlRole
	RoleRef string `json:"roleRef"`

	Subjects []rbacv1.Subject `json:"subjects"`

	Store Store `json:"store"`
}

type MysqlRoleBindingPhase string

type MysqlRoleBindingStatus struct {
	// observedGeneration is the most recent generation observed for this MysqlRoleBinding. It corresponds to the
	// MysqlRoleBinding's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// contains lease info of the credentials
	Lease LeaseData `json:"lease,omitempty"`

	// Specifies the phase of the MysqlRoleBinding
	Phase MysqlRoleBindingPhase `json:"phase,omitempty"`

	// Represents the latest available observations of a MysqlRoleBinding current state.
	Conditions []MysqlRoleBindingCondition `json:"conditions,omitempty"`
}

// MysqlRoleBindingCondition describes the state of a MysqlRoleBinding at a certain point.
type MysqlRoleBindingCondition struct {
	// Type of MysqlRoleBinding condition.
	Type string `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status,omitempty"`

	// The reason for the condition's.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MysqlRoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of MysqlRoleBinding objects
	Items []MysqlRoleBinding `json:"items,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MongodbRole
type MongodbRole struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MongodbRoleSpec   `json:"spec,omitempty"`
	Status            MongodbRoleStatus `json:"status,omitempty"`
}

// MongodbRoleSpec contains connection information, Mongodb role info etc
type MongodbRoleSpec struct {
	Provider *ProviderSpec             `json:"provider"`
	Database *DatabaseConfigForMongodb `json:"database,omitempty"`

	// links:
	// 	- https://www.vaultproject.io/api/secret/databases/index.html
	//	- https://www.vaultproject.io/api/secret/databases/mongodb.html

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

	// https://www.vaultproject.io/api/secret/databases/Mongodb-maria.html#creation_statements
	// Specifies the database statements executed to create and configure a user.
	CreationStatements []string `json:"creationStatements"`

	// https://www.vaultproject.io/api/secret/databases/Mongodb-maria.html#revocation_statements
	// Specifies the database statements to be executed to revoke a user.
	RevocationStatements []string `json:"revocationStatements,omitempty"`
}

// https://www.vaultproject.io/api/secret/databases/index.html
// https://www.vaultproject.io/api/secret/databases/mongodb.html#configure-connection
// DatabaseConfigForMongodb contains database connection config
type DatabaseConfigForMongodb struct {
	// Specifies the name for this database connection
	Name string `json:"name"`

	// Specifies the name of the plugin to use for this connection.
	// Default plugin:
	//	- for postgres: postgresql-database-plugin
	//  - for mysql: mysql-database-plugin
	PluginName string `json:"pluginName,omitempty"`

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

	// Specifies the MongoDB write concern. This is set for the entirety
	// of the session, maintained for the lifecycle of the plugin process.
	WriteConcern string `json:"writeConcern,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MongodbRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of MongodbRole objects
	Items []MongodbRole `json:"items,omitempty"`
}

type MongodbRolePhase string

type MongodbRoleStatus struct {
	Phase MongodbRolePhase `json:"phase,omitempty"`

	// observedGeneration is the most recent generation observed for this MongodbRole. It corresponds to the
	// MongodbRole's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Represents the latest available observations of a MongodbRole current state.
	Conditions []MongodbRoleCondition `json:"conditions,omitempty"`
}

// MongodbRoleCondition describes the state of a MongodbRole at a certain point.
type MongodbRoleCondition struct {
	// Type of MongodbRole condition.
	Type string `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status,omitempty"`

	// The reason for the condition's.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MongodbRoleBinding binds mongodb credential to user
type MongodbRoleBinding struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MongodbRoleBindingSpec   `json:"spec,omitempty"`
	Status            MongodbRoleBindingStatus `json:"status,omitempty"`
}

type MongodbRoleBindingSpec struct {
	// Specifies the name of the MongodbRole
	RoleRef string `json:"roleRef"`

	Subjects []rbacv1.Subject `json:"subjects"`

	Store Store `json:"store"`
}

type MongodbRoleBindingPhase string

type MongodbRoleBindingStatus struct {
	// observedGeneration is the most recent generation observed for this MongodbRoleBinding. It corresponds to the
	// MongodbRoleBinding's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// contains lease info of the credentials
	Lease LeaseData `json:"lease,omitempty"`

	// Specifies the phase of the MongodbRoleBinding
	Phase MongodbRoleBindingPhase `json:"phase,omitempty"`

	// Represents the latest available observations of a MongodbRoleBinding current state.
	Conditions []MongodbRoleBindingCondition `json:"conditions,omitempty"`
}

// MongodbRoleBindingCondition describes the state of a MongodbRoleBinding at a certain point.
type MongodbRoleBindingCondition struct {
	// Type of MongodbRoleBinding condition.
	Type string `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status,omitempty"`

	// The reason for the condition's.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MongodbRoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of MongodbRoleBinding objects
	Items []MongodbRoleBinding `json:"items,omitempty"`
}
