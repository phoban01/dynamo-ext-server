package pivot

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func oldDevice(name, uid, desc string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "inventory.example.com/v1",
		"kind":       "Device",
		"metadata": map[string]any{
			"name":        name,
			"uid":         uid,
			"labels":      map[string]any{"kind": "gpu"},
			"annotations": map[string]any{"team": "ml", lastApplied: "{}"},
		},
		"spec":   map[string]any{"description": desc},
		"status": map[string]any{"owner": "someone"},
	}}
	return u
}

func testSource(t *testing.T) Source {
	t.Helper()
	s, err := ParseSource("inventory.example.com/v1/devices", "spec.description")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

//= spec/solas.md#9-1-mapping
//= type=test
//# The Device MUST get the annotations of the old object, except
//# `kubectl.kubernetes.io/last-applied-configuration`.

//= spec/solas.md#9-1-mapping
//= type=test
//# The pivot MUST NOT copy the status of the old object.

func TestToDevice(t *testing.T) {
	d, err := ToDevice(oldDevice("gpu-1", "u1", "an A100"), testSource(t))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "gpu-1" || d.Labels["kind"] != "gpu" || d.Spec.Description != "an A100" {
		t.Errorf("device = %+v", d)
	}
	if d.Annotations["team"] != "ml" {
		t.Error("annotation team was not copied")
	}
	if _, ok := d.Annotations[lastApplied]; ok {
		t.Error("the kubectl last-applied annotation was copied")
	}
	if got := d.Annotations[AnnotationPivotedFrom]; got != "inventory.example.com/devices/u1" {
		t.Errorf("pivoted-from = %q", got)
	}
	if d.Status.ClaimRef != nil || d.Status.FencingToken != 0 {
		t.Error("status was copied; a pivoted device starts free")
	}
}

func TestParseSource(t *testing.T) {
	for _, bad := range []string{"", "devices", "g/v", "g//r"} {
		if _, err := ParseSource(bad, ""); err == nil {
			t.Errorf("ParseSource(%q) accepted it", bad)
		}
	}
	s, err := ParseSource("/v1/devices", "")
	if err != nil || s.Resource.Group != "" || len(s.DescriptionPath) != 0 {
		t.Errorf("core group: %+v, %v", s, err)
	}
}
