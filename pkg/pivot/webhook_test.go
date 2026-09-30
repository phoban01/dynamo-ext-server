package pivot

import (
	"bytes"
	"errors"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	solasv1alpha1 "github.com/phoban01/solas/pkg/apis/solas/v1alpha1"
)

func request(t *testing.T, op admissionv1.Operation, name, desc string) *admissionv1.AdmissionRequest {
	t.Helper()
	req := &admissionv1.AdmissionRequest{UID: types.UID("req-" + name), Operation: op, Name: name}
	if op != admissionv1.Delete {
		o := oldDevice(name, "", desc)
		raw, err := o.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		req.Object = runtime.RawExtension{Raw: raw}
	}
	return req
}

func newWebhook(t *testing.T, devices ...client.Object) *Webhook {
	t.Helper()
	return &Webhook{Pivot: newPivot(t, nil, devices...)}
}

//= spec/solas.md#9-4-webhook
//= type=test
//# On a create or an update, the webhook MUST write the Device before it
//# allows the request.

//= spec/solas.md#9-4-webhook
//= type=test
//# It MUST then add `solas.dev/pivoted-to` to the old object with a patch.

func TestWebhookCreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	w := newWebhook(t)
	resp := w.Admit(ctx, request(t, admissionv1.Create, "gpu-1", "first"))
	if !resp.Allowed || resp.PatchType == nil {
		t.Fatalf("create: %+v", resp)
	}
	var patch []map[string]any
	if err := json.Unmarshal(resp.Patch, &patch); err != nil || len(patch) != 1 ||
		patch[0]["path"] != "/metadata/annotations/solas.dev~1pivoted-to" || patch[0]["value"] != "solas.dev/devices/gpu-1" {
		t.Errorf("patch = %s", resp.Patch)
	}
	if d := device(t, w.Pivot, "gpu-1"); d.Spec.Description != "first" {
		t.Errorf("description = %q", d.Spec.Description)
	}
	if resp := w.Admit(ctx, request(t, admissionv1.Update, "gpu-1", "second")); !resp.Allowed {
		t.Fatalf("update: %+v", resp.Result)
	}
	if d := device(t, w.Pivot, "gpu-1"); d.Spec.Description != "second" {
		t.Errorf("after update description = %q", d.Spec.Description)
	}
}

//= spec/solas.md#9-4-webhook
//= type=test
//# On a dry-run request, the webhook MUST NOT write the Device.

func TestWebhookDryRun(t *testing.T) {
	ctx := context.Background()
	w := newWebhook(t)
	req := request(t, admissionv1.Create, "gpu-1", "first")
	req.DryRun = ptr.To(true)
	if resp := w.Admit(ctx, req); !resp.Allowed {
		t.Fatalf("dry run: %+v", resp.Result)
	}
	var list solasv1alpha1.DeviceList
	if err := w.Pivot.Solas.List(ctx, &list); err != nil || len(list.Items) != 0 {
		t.Errorf("dry run wrote %d Devices, %v", len(list.Items), err)
	}
}

func TestWebhookDeniesAConflict(t *testing.T) {
	other := &solasv1alpha1.Device{ObjectMeta: metav1.ObjectMeta{Name: "gpu-1"}}
	w := newWebhook(t, other)
	resp := w.Admit(context.Background(), request(t, admissionv1.Create, "gpu-1", "first"))
	if resp.Allowed || resp.Result.Code != http.StatusConflict {
		t.Errorf("create over a foreign Device: %+v", resp)
	}
}

//= spec/solas.md#9-4-webhook
//= type=test
//# If solas rejects the delete of the Device, the webhook MUST deny the
//# delete of the old object.

func TestWebhookDelete(t *testing.T) {
	ctx := context.Background()
	w := newWebhook(t)
	for _, name := range []string{"gpu-1", "gpu-2"} {
		if resp := w.Admit(ctx, request(t, admissionv1.Create, name, "x")); !resp.Allowed {
			t.Fatal(resp.Result)
		}
	}
	if resp := w.Admit(ctx, request(t, admissionv1.Delete, "gpu-1", "")); !resp.Allowed {
		t.Fatalf("delete gpu-1: %+v", resp.Result)
	}
	var d solasv1alpha1.Device
	if err := w.Pivot.Solas.Get(ctx, client.ObjectKey{Name: "gpu-1"}, &d); err == nil {
		t.Error("the Device of a deleted old object still exists")
	}

	// solas rejects the delete of a bound device with 409; the fake client
	// here plays solas.
	w.Pivot.Solas = conflictOnDelete{w.Pivot.Solas}
	resp := w.Admit(ctx, request(t, admissionv1.Delete, "gpu-2", ""))
	if resp.Allowed || resp.Result.Code != http.StatusConflict {
		t.Errorf("delete of a bound device: %+v", resp)
	}
}

// conflictOnDelete answers every delete with 409, as solas does for a
// bound device.
type conflictOnDelete struct{ client.Client }

func (c conflictOnDelete) Delete(_ context.Context, obj client.Object, _ ...client.DeleteOption) error {
	return newConflict(obj.GetName())
}

func TestServeHTTP(t *testing.T) {
	w := newWebhook(t)
	review := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request:  request(t, admissionv1.Create, "gpu-1", "first"),
	}
	body, _ := json.Marshal(review)
	rr := httptest.NewRecorder()
	w.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body)))
	var out admissionv1.AdmissionReview
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil || out.Response == nil ||
		!out.Response.Allowed || out.Response.UID != "req-gpu-1" {
		t.Errorf("response = %s", rr.Body.String())
	}
}

func newConflict(name string) error {
	return apierrors.NewConflict(solasv1alpha1.Resource("devices"), name,
		errors.New("device is bound"))
}

