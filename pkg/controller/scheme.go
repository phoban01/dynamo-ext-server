// Package controller builds solas-controller: the member manager, the
// claim reconciler, and the sweeper, spec sections 6 to 8.
package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	claimsv1alpha1 "github.com/phoban01/solas/pkg/apis/claims/v1alpha1"
	solasv1alpha1 "github.com/phoban01/solas/pkg/apis/solas/v1alpha1"
)

// NewScheme returns a scheme with the core types and both solas groups.
func NewScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(solasv1alpha1.AddToScheme(s))
	utilruntime.Must(claimsv1alpha1.AddToScheme(s))
	return s
}
