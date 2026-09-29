package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// DefaultLeaseDurationSeconds is the default lease duration D, spec 7.3.
const DefaultLeaseDurationSeconds = 30

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

// SetDefaults_MemberSpec sets the lease duration when it is not set.
func SetDefaults_MemberSpec(obj *MemberSpec) {
	if obj.LeaseDurationSeconds == 0 {
		obj.LeaseDurationSeconds = DefaultLeaseDurationSeconds
	}
}
