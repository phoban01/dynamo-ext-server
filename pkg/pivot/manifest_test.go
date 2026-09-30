package pivot

import (
	"os"
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"sigs.k8s.io/yaml"
)

//= spec/solas.md#9-4-webhook
//= type=test
//# The webhook configuration MUST use `failurePolicy: Fail`.

// TestWebhookManifest checks the shipped webhook configuration.
func TestWebhookManifest(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/pivot/webhook.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg admissionregistrationv1.MutatingWebhookConfiguration
	if err := yaml.UnmarshalStrict(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Webhooks) != 1 {
		t.Fatalf("webhooks = %d, want 1", len(cfg.Webhooks))
	}
	w := cfg.Webhooks[0]
	if w.FailurePolicy == nil || *w.FailurePolicy != admissionregistrationv1.Fail {
		t.Errorf("failurePolicy = %v, want Fail", w.FailurePolicy)
	}
	if w.SideEffects == nil || *w.SideEffects != admissionregistrationv1.SideEffectClassNoneOnDryRun {
		t.Errorf("sideEffects = %v, want NoneOnDryRun", w.SideEffects)
	}
	ops := map[admissionregistrationv1.OperationType]bool{}
	for _, r := range w.Rules {
		for _, op := range r.Operations {
			ops[op] = true
		}
	}
	for _, op := range []admissionregistrationv1.OperationType{"CREATE", "UPDATE", "DELETE"} {
		if !ops[op] {
			t.Errorf("the webhook does not handle %s", op)
		}
	}
}
