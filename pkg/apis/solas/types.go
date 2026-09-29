package solas

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Device is a leasable device. It is global: every member cluster sees
// the same devices.
type Device struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	Spec   DeviceSpec
	Status DeviceStatus
}

// DeviceSpec describes a device.
type DeviceSpec struct {
	// Description holds free text about the device.
	Description string
}

// DeviceStatus shows which claim holds the device.
type DeviceStatus struct {
	// ClaimRef names the holder. A device with no ClaimRef is free.
	ClaimRef *ClaimRef
	// FencingToken is the fencing token of the last bind. The server sets
	// it, spec 5.3.
	FencingToken int64
}

// ClaimRef names the claim that holds a device, spec 5.2.
type ClaimRef struct {
	// Member is the name of the member cluster of the claim.
	Member string
	// MemberUID is the UID of the Member when it wrote the ref.
	MemberUID types.UID
	// Namespace is the namespace of the claim.
	Namespace string
	// Name is the name of the claim.
	Name string
	// UID is the UID of the claim.
	UID types.UID
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeviceList is a list of devices.
type DeviceList struct {
	metav1.TypeMeta
	metav1.ListMeta

	Items []Device
}

// MemberPhase is the phase of a member cluster.
type MemberPhase string

// Member phases, spec 7.1.
const (
	MemberActive   MemberPhase = "Active"
	MemberDraining MemberPhase = "Draining"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Member is one member cluster and its lease. Its name is the cluster ID.
type Member struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	Spec   MemberSpec
	Status MemberStatus
}

// MemberSpec holds the lease settings of a member.
type MemberSpec struct {
	// LeaseDurationSeconds is the lease duration D.
	LeaseDurationSeconds int32
}

// MemberStatus holds the state of the lease.
type MemberStatus struct {
	// RenewTime is the time of the last renew, by the holder's clock.
	// Observers do not compare it with their own clocks, spec 8.1.
	RenewTime *metav1.MicroTime
	// Phase is Active or Draining.
	Phase MemberPhase
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MemberList is a list of members.
type MemberList struct {
	metav1.TypeMeta
	metav1.ListMeta

	Items []Member
}
