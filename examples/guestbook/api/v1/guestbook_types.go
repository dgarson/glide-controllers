// Package v1 contains API Schema definitions for the guestbook v1 API group.
// This is a sample CRD used to demonstrate the Streamline framework.
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GuestbookSpec defines the desired state of Guestbook.
type GuestbookSpec struct {
	// Message is the greeting message to display.
	// +kubebuilder:validation:Required
	Message string `json:"message"`

	// Replicas is the number of guestbook instances to run.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Replicas int32 `json:"replicas,omitempty"`

	// ExternalResource simulates an external dependency that requires cleanup.
	// When set, the controller will simulate creating/deleting an external resource.
	ExternalResource string `json:"externalResource,omitempty"`
}

// GuestbookStatus defines the observed state of Guestbook.
type GuestbookStatus struct {
	// Phase represents the current lifecycle phase of the Guestbook.
	// +kubebuilder:validation:Enum=Pending;Running;Failed;Terminating
	Phase string `json:"phase,omitempty"`

	// ReadyReplicas is the number of ready guestbook instances.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Message provides additional status information.
	Message string `json:"message,omitempty"`

	// ExternalResourceCreated indicates whether the external resource was created.
	ExternalResourceCreated bool `json:"externalResourceCreated,omitempty"`

	// LastUpdated is the timestamp of the last status update.
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Guestbook is the Schema for the guestbooks API.
// It represents a simple guestbook application for demonstration purposes.
type Guestbook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GuestbookSpec   `json:"spec,omitempty"`
	Status GuestbookStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GuestbookList contains a list of Guestbook.
type GuestbookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Guestbook `json:"items"`
}
