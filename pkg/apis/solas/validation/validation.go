// Package validation checks Device and Member objects.
package validation

import (
	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/phoban01/solas/pkg/apis/solas"
)

var (
	metaPath   = field.NewPath("metadata")
	specPath   = field.NewPath("spec")
	statusPath = field.NewPath("status")
)

// ValidateDevice checks a new device.
func ValidateDevice(d *solas.Device) field.ErrorList {
	errs := genericvalidation.ValidateObjectMeta(&d.ObjectMeta, false, genericvalidation.NameIsDNSSubdomain, metaPath)
	return append(errs, validateClaimRef(d.Status.ClaimRef, statusPath.Child("claimRef"))...)
}

// ValidateDeviceUpdate checks an update of a device.
func ValidateDeviceUpdate(d, old *solas.Device) field.ErrorList {
	errs := genericvalidation.ValidateObjectMetaUpdate(&d.ObjectMeta, &old.ObjectMeta, metaPath)
	return append(errs, ValidateDevice(d)...)
}

// ValidateDeviceStatusUpdate checks an update of the status of a device.
func ValidateDeviceStatusUpdate(d, old *solas.Device) field.ErrorList {
	errs := genericvalidation.ValidateObjectMetaUpdate(&d.ObjectMeta, &old.ObjectMeta, metaPath)
	path := statusPath.Child("claimRef")
	errs = append(errs, validateClaimRef(d.Status.ClaimRef, path)...)
	//= spec/solas.md#5-3-status-updates
	//# The server MUST reject a status update that changes `claimRef` from one
	//# holder to a different holder.

	//= spec/solas.md#5-3-status-updates
	//# A holder is different when any field of `claimRef` differs.

	//= spec/solas.md#5-3-status-updates
	//# To move a device, a client MUST first clear `claimRef` and then set it
	//# in a second update.
	if old.Status.ClaimRef != nil && d.Status.ClaimRef != nil && *old.Status.ClaimRef != *d.Status.ClaimRef {
		errs = append(errs, field.Forbidden(path,
			"cannot change the holder of a bound device; clear claimRef first, then set it"))
	}
	return errs
}

//= spec/solas.md#5-3-status-updates
//# The server MUST reject a status update that sets a `claimRef` with an
//# empty field.

//= spec/solas.md#5-2-claim-reference
//# A `claimRef` MUST hold the member name, the member UID, the claim
//# namespace, the claim name, and the claim UID.

func validateClaimRef(ref *solas.ClaimRef, path *field.Path) field.ErrorList {
	if ref == nil {
		return nil
	}
	var errs field.ErrorList
	for name, value := range map[string]string{
		"member":    ref.Member,
		"memberUID": string(ref.MemberUID),
		"namespace": ref.Namespace,
		"name":      ref.Name,
		"uid":       string(ref.UID),
	} {
		if value == "" {
			errs = append(errs, field.Required(path.Child(name), ""))
		}
	}
	return errs
}

// ValidateMember checks a new member.
func ValidateMember(m *solas.Member) field.ErrorList {
	errs := genericvalidation.ValidateObjectMeta(&m.ObjectMeta, false, genericvalidation.NameIsDNSSubdomain, metaPath)
	//= spec/solas.md#7-1-resource
	//# `Member.spec.leaseDurationSeconds` MUST hold the lease duration `D`.
	if m.Spec.LeaseDurationSeconds <= 0 {
		errs = append(errs, field.Invalid(specPath.Child("leaseDurationSeconds"), m.Spec.LeaseDurationSeconds, "must be greater than 0"))
	}
	return append(errs, validateMemberStatus(m)...)
}

// ValidateMemberUpdate checks an update of a member.
func ValidateMemberUpdate(m, old *solas.Member) field.ErrorList {
	errs := genericvalidation.ValidateObjectMetaUpdate(&m.ObjectMeta, &old.ObjectMeta, metaPath)
	return append(errs, ValidateMember(m)...)
}

// ValidateMemberStatusUpdate checks an update of the status of a member.
func ValidateMemberStatusUpdate(m, old *solas.Member) field.ErrorList {
	errs := genericvalidation.ValidateObjectMetaUpdate(&m.ObjectMeta, &old.ObjectMeta, metaPath)
	return append(errs, validateMemberStatus(m)...)
}

func validateMemberStatus(m *solas.Member) field.ErrorList {
	//= spec/solas.md#7-1-resource
	//# `Member.status.phase` MUST be `Active` or `Draining`.
	switch m.Status.Phase {
	case solas.MemberActive, solas.MemberDraining:
		return nil
	}
	return field.ErrorList{field.NotSupported(statusPath.Child("phase"), m.Status.Phase,
		[]string{string(solas.MemberActive), string(solas.MemberDraining)})}
}
