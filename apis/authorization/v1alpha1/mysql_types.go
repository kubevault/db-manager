package v1alpha1

import (
	"github.com/appscode/go/encoding/json/types"
	"k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceKindMySQLRole = "MySQLRole"
	ResourceMySQLRole     = "mysqlrole"
	ResourceMySQLRoles    = "mysqlroles"

	ResourceKindMySQLRoleBinding = "MySQLRoleBinding"
	ResourceMySQLRoleBinding     = "mysqlrolebinding"
	ResourceMySQLRoleBindings    = "mysqlrolebindings"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MySQLRole
type MySQLRole struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MySQLRoleSpec   `json:"spec,omitempty"`
	Status            MySQLRoleStatus `json:"status,omitempty"`
}

// MySQLRoleSpec contains connection information, mysql role info etc
type MySQLRoleSpec struct {
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
	//  - for mysql: mysql-database-plugin
	PluginName string `json:"pluginName,omitempty"`

	// Specifies the MySQL DSN. This field can be templated and supports
	// passing the username and password parameters in the following format {{field_name}}.
	// A templated connection URL is required when using root credential rotation.
	// e.g. {{username}}:{{password}}@tcp(127.0.0.1:3306)/
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

type MySQLRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of MySQLRole objects
	Items []MySQLRole `json:"items,omitempty"`
}

type MySQLRolePhase string

type MySQLRoleStatus struct {
	Phase MySQLRolePhase `json:"phase,omitempty"`

	// observedGeneration is the most recent generation observed for this MySQLRole. It corresponds to the
	// MySQLRole's generation, which is updated on mutation by the API Server.
	ObservedGeneration *types.IntHash `json:"observedGeneration,omitempty"`

	// Represents the latest available observations of a MySQLRole current state.
	Conditions []MySQLRoleCondition `json:"conditions,omitempty"`
}

// MySQLRoleCondition describes the state of a MySQLRole at a certain point.
type MySQLRoleCondition struct {
	// Type of MySQLRole condition.
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

// MySQLRoleBinding binds mysql credential to user
type MySQLRoleBinding struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MySQLRoleBindingSpec   `json:"spec,omitempty"`
	Status            MySQLRoleBindingStatus `json:"status,omitempty"`
}

type MySQLRoleBindingSpec struct {
	// Specifies the name of the MySQLRole
	RoleRef string `json:"roleRef"`

	Subjects []rbacv1.Subject `json:"subjects"`

	Store Store `json:"store"`
}

type MySQLRoleBindingPhase string

type MySQLRoleBindingStatus struct {
	// observedGeneration is the most recent generation observed for this MySQLRoleBinding. It corresponds to the
	// MySQLRoleBinding's generation, which is updated on mutation by the API Server.
	ObservedGeneration *types.IntHash `json:"observedGeneration,omitempty"`

	// contains lease info of the credentials
	Lease LeaseData `json:"lease,omitempty"`

	// Specifies the phase of the MySQLRoleBinding
	Phase MySQLRoleBindingPhase `json:"phase,omitempty"`

	// Represents the latest available observations of a MySQLRoleBinding current state.
	Conditions []MySQLRoleBindingCondition `json:"conditions,omitempty"`
}

// MySQLRoleBindingCondition describes the state of a MySQLRoleBinding at a certain point.
type MySQLRoleBindingCondition struct {
	// Type of MySQLRoleBinding condition.
	Type string `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status,omitempty"`

	// The reason for the condition's.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MySQLRoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of MySQLRoleBinding objects
	Items []MySQLRoleBinding `json:"items,omitempty"`
}
