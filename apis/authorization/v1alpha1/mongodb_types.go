package v1alpha1

import (
	"k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceKindMongodbRole = "MongodbRole"
	ResourceMongodbRole     = "mongodbrole"
	ResourceMongodbRoles    = "mongodbroles"

	ResourceKindMongodbRoleBinding = "MongodbRoleBinding"
	ResourceMongodbRoleBinding     = "mongodbrolebinding"
	ResourceMongodbRoleBindings    = "mongodbrolebindings"
)

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
	//  - for mongodb: mongodb-database-plugin
	PluginName string `json:"pluginName,omitempty"`

	// pecifies the MongoDB standard connection string (URI). This field can be templated and supports
	// passing the username and password parameters in the following format {{field_name}}.
	// A templated connection URL is required when using root credential rotation.
	// e.g. mongodb://{{username}}:{{password}}@mongodb.acme.com:27017/admin?ssl=true
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
