package device

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/phoban01/solas/pkg/apis/solas"
)

var ref1 = &solas.ClaimRef{Member: "a", MemberUID: "mu", Namespace: "ns", Name: "c1", UID: "u1"}

func dev(desc string, ref *solas.ClaimRef) *solas.Device {
	return &solas.Device{
		ObjectMeta: metav1.ObjectMeta{Name: "d1", ResourceVersion: "2", Labels: map[string]string{"k": desc}},
		Spec:       solas.DeviceSpec{Description: desc},
		Status:     solas.DeviceStatus{ClaimRef: ref},
	}
}

func TestStrategiesAreConditional(t *testing.T) {
	s := NewStrategy(runtime.NewScheme())
	if s.AllowUnconditionalUpdate(context.Background()) || NewStatusStrategy(s).AllowUnconditionalUpdate(context.Background()) {
		t.Error("device updates must carry a resource version")
	}
	if s.NamespaceScoped() {
		t.Error("Device must be cluster-scoped")
	}
}

func TestCreateStartsFree(t *testing.T) {
	d := dev("x", ref1)
	NewStrategy(runtime.NewScheme()).PrepareForCreate(context.Background(), d)
	if d.Status.ClaimRef != nil {
		t.Error("a new device must have no claimRef")
	}
}

//= spec/solas.md#5-3-status-updates
//= type=test
//# A spec update MUST NOT change `status`.

func TestSpecUpdateKeepsStatus(t *testing.T) {
	old, d := dev("old", ref1), dev("new", nil)
	NewStrategy(runtime.NewScheme()).PrepareForUpdate(context.Background(), d, old)
	if d.Status.ClaimRef == nil || d.Spec.Description != "new" {
		t.Errorf("after spec update: spec %q, claimRef %v", d.Spec.Description, d.Status.ClaimRef)
	}
}

//= spec/solas.md#5-3-status-updates
//= type=test
//# A status update MUST NOT change `spec`.

func TestStatusUpdateKeepsSpec(t *testing.T) {
	old, d := dev("old", nil), dev("new", ref1)
	NewStatusStrategy(NewStrategy(runtime.NewScheme())).PrepareForUpdate(context.Background(), d, old)
	if d.Spec.Description != "old" || d.Labels["k"] != "old" || d.Status.ClaimRef == nil {
		t.Errorf("after status update: spec %q, label %q, claimRef %v", d.Spec.Description, d.Labels["k"], d.Status.ClaimRef)
	}
}

func TestStatusUpdateRejectsNewHolder(t *testing.T) {
	ss := NewStatusStrategy(NewStrategy(runtime.NewScheme()))
	other := *ref1
	other.Name, other.UID = "c2", "u2"
	if errs := ss.ValidateUpdate(context.Background(), dev("x", &other), dev("x", ref1)); len(errs) == 0 {
		t.Error("status strategy accepted a change of holder")
	}
	if errs := ss.ValidateUpdate(context.Background(), dev("x", ref1), dev("x", nil)); len(errs) != 0 {
		t.Errorf("status strategy rejected a bind of a free device: %v", errs)
	}
}
