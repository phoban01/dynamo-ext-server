package member

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/phoban01/solas/pkg/apis/solas"
)

func TestMemberStrategy(t *testing.T) {
	ctx := context.Background()
	s := NewStrategy(runtime.NewScheme())
	if s.NamespaceScoped() {
		t.Error("Member must be cluster-scoped")
	}
	if s.AllowUnconditionalUpdate(ctx) || NewStatusStrategy(s).AllowUnconditionalUpdate(ctx) {
		t.Error("member updates must carry a resource version")
	}

	m := &solas.Member{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Spec: solas.MemberSpec{LeaseDurationSeconds: 30}}
	s.PrepareForCreate(ctx, m)
	if m.Status.Phase != solas.MemberActive {
		t.Errorf("phase after create = %q, want Active", m.Status.Phase)
	}
	if errs := s.Validate(ctx, m); len(errs) != 0 {
		t.Errorf("validate: %v", errs)
	}

	renewed := m.DeepCopy()
	renewed.Spec.LeaseDurationSeconds = 99
	now := metav1.NowMicro()
	renewed.Status.RenewTime = &now
	NewStatusStrategy(s).PrepareForUpdate(ctx, renewed, m)
	if renewed.Spec.LeaseDurationSeconds != 30 || renewed.Status.RenewTime == nil {
		t.Errorf("status update: lease %d, renewTime %v", renewed.Spec.LeaseDurationSeconds, renewed.Status.RenewTime)
	}
}
