// Package install registers the solas.dev API group in a scheme.
package install

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"github.com/phoban01/solas/pkg/apis/solas"
	"github.com/phoban01/solas/pkg/apis/solas/v1alpha1"
)

// Install adds the internal and v1alpha1 types to the scheme.
func Install(scheme *runtime.Scheme) {
	utilruntime.Must(solas.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	utilruntime.Must(scheme.SetVersionPriority(v1alpha1.SchemeGroupVersion))
}
