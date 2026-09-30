package claim

import (
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	claimsv1alpha1 "github.com/phoban01/solas/pkg/apis/claims/v1alpha1"
	solasv1alpha1 "github.com/phoban01/solas/pkg/apis/solas/v1alpha1"
)

// celCostLimit bounds the work of one expression on one device.
const celCostLimit = 100000

// matcher says whether a device matches a claim.
type matcher struct {
	labels labels.Selector
	prog   cel.Program
}

// programs caches compiled CEL expressions by their text.
type programs struct {
	mu    sync.Mutex
	env   *cel.Env
	cache map[string]cel.Program
}

func (p *programs) compile(expr string) (cel.Program, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if prog, ok := p.cache[expr]; ok {
		return prog, nil
	}
	if p.env == nil {
		env, err := cel.NewEnv(cel.Variable("device", cel.DynType))
		if err != nil {
			return nil, err
		}
		p.env, p.cache = env, map[string]cel.Program{}
	}
	ast, iss := p.env.Compile(expr)
	if iss.Err() != nil {
		return nil, iss.Err()
	}
	if ast.OutputType() != cel.BoolType && ast.OutputType() != cel.DynType {
		return nil, fmt.Errorf("the expression gives %s, not a bool", ast.OutputType())
	}
	prog, err := p.env.Program(ast, cel.CostLimit(celCostLimit))
	if err != nil {
		return nil, err
	}
	p.cache[expr] = prog
	return prog, nil
}

// newMatcher builds the matcher of a claim.
func (r *Reconciler) newMatcher(claim *claimsv1alpha1.DeviceClaim) (*matcher, error) {
	m := &matcher{labels: labels.Everything()}
	sel := claim.Spec.Selector
	if sel == nil {
		return m, nil
	}
	var err error
	if m.labels, err = metav1.LabelSelectorAsSelector(&sel.LabelSelector); err != nil {
		return nil, fmt.Errorf("label selector: %w", err)
	}
	if sel.CEL != "" {
		if m.prog, err = r.programs.compile(sel.CEL); err != nil {
			return nil, fmt.Errorf("cel: %w", err)
		}
	}
	return m, nil
}

//= spec/solas.md#10-3-selection
//# A device matches a claim only when the label selector and the CEL
//# expression both match it.

// Matches reports whether d matches.
func (m *matcher) Matches(d *solasv1alpha1.Device) bool {
	if !m.labels.Matches(labels.Set(d.Labels)) {
		return false
	}
	if m.prog == nil {
		return true
	}
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(d)
	if err != nil {
		return false
	}
	out, _, err := m.prog.Eval(map[string]any{"device": obj})
	//= spec/solas.md#10-3-selection
	//# If the expression fails at run time for a device, that device MUST NOT
	//# match.
	if err != nil {
		return false
	}
	return out == types.True
}
