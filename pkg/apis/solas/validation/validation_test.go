package validation

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/phoban01/solas/pkg/apis/solas"
)

func ref(claim string) *solas.ClaimRef {
	return &solas.ClaimRef{Member: "a", MemberUID: "mu", Namespace: "ns", Name: claim, UID: types.UID("u-" + claim)}
}

func device(r *solas.ClaimRef) *solas.Device {
	return &solas.Device{
		ObjectMeta: metav1.ObjectMeta{Name: "d1", ResourceVersion: "2"},
		Status:     solas.DeviceStatus{ClaimRef: r},
	}
}

//= spec/solas.md#5-3-status-updates
//= type=test
//# The server MUST reject a status update that sets a `claimRef` with an
//# empty field.

func TestClaimRefNeedsEveryField(t *testing.T) {
	if errs := ValidateDevice(device(ref("c1"))); len(errs) != 0 {
		t.Fatalf("full claimRef: %v", errs)
	}
	for _, clear := range []func(*solas.ClaimRef){
		func(r *solas.ClaimRef) { r.Member = "" },
		func(r *solas.ClaimRef) { r.MemberUID = "" },
		func(r *solas.ClaimRef) { r.Namespace = "" },
		func(r *solas.ClaimRef) { r.Name = "" },
		func(r *solas.ClaimRef) { r.UID = "" },
	} {
		r := ref("c1")
		clear(r)
		if errs := ValidateDeviceStatusUpdate(device(r), device(nil)); len(errs) != 1 {
			t.Errorf("claimRef %+v: got %d errors, want 1: %v", *r, len(errs), errs)
		}
	}
}

//= spec/solas.md#5-3-status-updates
//= type=test
//# The server MUST reject a status update that changes `claimRef` from one
//# holder to a different holder.

func TestHolderToHolderIsRejected(t *testing.T) {
	tests := []struct {
		name     string
		old, new *solas.ClaimRef
		wantErr  bool
	}{
		{"bind a free device", nil, ref("c1"), false},
		{"release", ref("c1"), nil, false},
		{"keep the same holder", ref("c1"), ref("c1"), false},
		{"move to another claim", ref("c1"), ref("c2"), true},
		{"change only the member UID", ref("c1"), func() *solas.ClaimRef { r := ref("c1"); r.MemberUID = "other"; return r }(), true},
	}
	for _, tt := range tests {
		errs := ValidateDeviceStatusUpdate(device(tt.new), device(tt.old))
		if (len(errs) > 0) != tt.wantErr {
			t.Errorf("%s: errors = %v, wantErr %v", tt.name, errs, tt.wantErr)
		}
	}
}

func TestMember(t *testing.T) {
	good := &solas.Member{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       solas.MemberSpec{LeaseDurationSeconds: 30},
		Status:     solas.MemberStatus{Phase: solas.MemberActive},
	}
	if errs := ValidateMember(good); len(errs) != 0 {
		t.Fatalf("good member: %v", errs)
	}
	noLease := good.DeepCopy()
	noLease.Spec.LeaseDurationSeconds = 0
	if errs := ValidateMember(noLease); len(errs) != 1 {
		t.Errorf("lease 0: got %v, want 1 error", errs)
	}
	badPhase := good.DeepCopy()
	badPhase.Status.Phase = "Gone"
	if errs := ValidateMember(badPhase); len(errs) != 1 {
		t.Errorf("phase Gone: got %v, want 1 error", errs)
	}
}
