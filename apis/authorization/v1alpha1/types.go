package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceKindMessagingService = "MessagingService"
	ResourceMessagingService     = "messagingservice"
	ResourceMessagingServices    = "messagingservices"

	ResourceKindMessage = "Message"
	ResourceMessage     = "message"
	ResourceMessages    = "messages"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Notifier defines a Notifier database.
type MessagingService struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MessagingServiceSpec `json:"spec,omitempty"`
	//Status            NotifierStatus `json:"status,omitempty"`
}

type MessagingServiceSpec struct {
	// Number of instances to deploy for a Notifier database.
	Replicas *int32 `json:"replicas,omitempty"`

	// To whom notification will be sent
	To []string `json:"to,omitempty"`

	// How this notification will be sent
	Drive string `json:"drive,omitempty"`

	// Secret name to which credential data is provided to send notification
	CredentialSecretName string `json:"credentialSecretName,omitempty"`
}

//type NotifierStatus struct {
//	CreationTime *metav1.Time `json:"creationTime,omitempty"`
//	Reason       string       `json:"reason,omitempty"`
//}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MessagingServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a list of MessagingService TPR objects
	Items []MessagingService `json:"items,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Message struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MessageSpec   `json:"spec,omitempty"`
	Status            MessageStatus `json:"status,omitempty"`
}

type MessageSpec struct {
	Service string `json:"service,omitempty"`
	Message string `json:"message,omitempty"`
	Email   string `json:"email,omitempty"`
	Chat    string `json:"chat,omitempty"`
	Sms     string `json:"sms,omitempty"`
}

type MessageStatus struct {
	SentTimestamp *metav1.Time `json:"sentTimestamp,omitempty"`
	ErrorMessage  string       `json:"errorMessage,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MessageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a list of Notification TPR objects
	Items []Message `json:"items,omitempty"`
}
