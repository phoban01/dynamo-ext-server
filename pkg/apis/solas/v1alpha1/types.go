package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Device is a leasable device. It is global: every member cluster sees
// the same devices.
type Device struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceSpec   `json:"spec,omitempty"`
	Status DeviceStatus `json:"status,omitempty"`
}

// DeviceSpec describes a device.
type DeviceSpec struct {
	// Description holds free text about the device.
	// +optional
	Description string `json:"description,omitempty"`
}

// DeviceStatus shows which claim holds the device.
type DeviceStatus struct {
	// ClaimRef names the holder. A device with no claimRef is free.
	// +optional
	ClaimRef *ClaimRef `json:"claimRef,omitempty"`
}

// ClaimRef names the claim that holds a device.
type ClaimRef struct {
	// Member is the name of the member cluster of the claim.
	Member string `json:"member"`
	// MemberUID is the UID of the Member when it wrote the ref.
	MemberUID types.UID `json:"memberUID"`
	// Namespace is the namespace of the claim.
	Namespace string `json:"namespace"`
	// Name is the name of the claim.
	Name string `json:"name"`
	// UID is the UID of the claim.
	UID types.UID `json:"uid"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeviceList is a list of devices.
type DeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Device `json:"items"`
}

// MemberPhase is the phase of a member cluster.
type MemberPhase string

// Member phases.
const (
	MemberActive   MemberPhase = "Active"
	MemberDraining MemberPhase = "Draining"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Member is one member cluster and its lease. Its name is the cluster ID.
type Member struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemberSpec   `json:"spec,omitempty"`
	Status MemberStatus `json:"status,omitempty"`
}

// MemberSpec holds the lease settings of a member.
type MemberSpec struct {
	// LeaseDurationSeconds is the lease duration D. The default is 30.
	// +optional
	LeaseDurationSeconds int32 `json:"leaseDurationSeconds,omitempty"`
}

// MemberStatus holds the state of the lease.
type MemberStatus struct {
	// RenewTime is the time of the last renew, by the holder's clock.
	// +optional
	RenewTime *metav1.MicroTime `json:"renewTime,omitempty"`
	// Phase is Active or Draining.
	// +optional
	Phase MemberPhase `json:"phase,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MemberList is a list of members.
type MemberList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Member `json:"items"`
}
