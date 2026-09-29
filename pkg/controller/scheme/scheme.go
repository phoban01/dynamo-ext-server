// Package scheme holds the types that solas-controller reads and writes.
package scheme

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	claimsv1alpha1 "github.com/phoban01/solas/pkg/apis/claims/v1alpha1"
	solasv1alpha1 "github.com/phoban01/solas/pkg/apis/solas/v1alpha1"
)

// New returns a scheme with the core types and both solas groups.
func New() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(solasv1alpha1.AddToScheme(s))
	utilruntime.Must(claimsv1alpha1.AddToScheme(s))
	return s
}
