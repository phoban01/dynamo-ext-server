package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ClaimPhase is the phase of a claim, spec 6.5.
type ClaimPhase string

// Claim phases.
const (
	ClaimPending   ClaimPhase = "Pending"
	ClaimBound     ClaimPhase = "Bound"
	ClaimSuspended ClaimPhase = "Suspended"
	ClaimLost      ClaimPhase = "Lost"
)

// ReleaseFinalizer keeps a claim until its device is released, spec 6.3.
const ReleaseFinalizer = "solas.dev/release"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeviceClaim asks for one device. It lives in the etcd of its cluster.
type DeviceClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceClaimSpec   `json:"spec,omitempty"`
	Status DeviceClaimStatus `json:"status,omitempty"`
}

// DeviceClaimSpec says which devices the claim accepts.
type DeviceClaimSpec struct {
	// Selector matches Device labels. An empty selector matches every
	// device.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// DeviceClaimStatus shows the binding of the claim.
type DeviceClaimStatus struct {
	// Phase is Pending, Bound, Suspended, or Lost.
	// +optional
	Phase ClaimPhase `json:"phase,omitempty"`
	// DeviceName names the bound device.
	// +optional
	DeviceName string `json:"deviceName,omitempty"`
	// MemberUID is the UID of the Member that bound the claim.
	// +optional
	MemberUID types.UID `json:"memberUID,omitempty"`
	// FencingToken is the token of the bind. A workload presents it each
	// time it uses the device, spec 6.6.
	// +optional
	FencingToken int64 `json:"fencingToken,omitempty"`
	// Conditions give details of the phase.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeviceClaimList is a list of claims.
type DeviceClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []DeviceClaim `json:"items"`
}
