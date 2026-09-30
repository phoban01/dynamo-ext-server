package pivot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	solasv1alpha1 "github.com/phoban01/solas/pkg/apis/solas/v1alpha1"
)

//= spec/solas.md#9-4-webhook
//# The webhook MUST be a mutating admission webhook on the old resource,
//# for create, update, and delete.

//= spec/solas.md#9-4-webhook
//# The webhook configuration MUST use `failurePolicy: Fail`.
//
// deploy/pivot/webhook.yaml sets it, and manifest_test.go checks it.

// Webhook is the mutating admission webhook on the old resource. It keeps
// each Device in step with its old object while clients still write the
// old CRD.
type Webhook struct {
	Pivot *Pivot
}

// ServeHTTP handles one AdmissionReview.
func (w *Webhook) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&review); err != nil || review.Request == nil {
		http.Error(rw, "want an AdmissionReview", http.StatusBadRequest)
		return
	}
	resp := w.Admit(r.Context(), review.Request)
	resp.UID = review.Request.UID
	review.Response = resp
	review.Request = nil
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(&review)
}

// Admit decides one request.
func (w *Webhook) Admit(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	//= spec/solas.md#9-4-webhook
	//# On a dry-run request, the webhook MUST NOT write the Device.
	p := *w.Pivot
	p.DryRun = req.DryRun != nil && *req.DryRun

	switch req.Operation {
	case admissionv1.Create, admissionv1.Update:
		return w.write(ctx, &p, req)
	case admissionv1.Delete:
		return w.delete(ctx, &p, req)
	}
	return allow()
}

func (w *Webhook) write(ctx context.Context, p *Pivot, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var o unstructured.Unstructured
	if err := o.UnmarshalJSON(req.Object.Raw); err != nil {
		return deny(http.StatusBadRequest, "decode object: %v", err)
	}
	if o.GetName() == "" {
		return deny(http.StatusUnprocessableEntity, "the pivot needs a name; generateName is not supported")
	}
	//= spec/solas.md#9-4-webhook
	//# On a create or an update, the webhook MUST write the Device before it
	//# allows the request.
	result, err := p.upsert(ctx, &o)
	if err != nil {
		//= spec/solas.md#9-4-webhook
		//# If the webhook cannot write the Device, it MUST deny the request.
		return deny(http.StatusInternalServerError, "write solas Device %s: %v", o.GetName(), err)
	}
	if result == conflict {
		return deny(http.StatusConflict,
			"solas Device %s exists and is not pivoted from this object", o.GetName())
	}
	//= spec/solas.md#9-4-webhook
	//# It MUST then add `solas.dev/pivoted-to` to the old object with a patch.
	return patchAnnotation(&o)
}

func (w *Webhook) delete(ctx context.Context, p *Pivot, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var d solasv1alpha1.Device
	err := p.Solas.Get(ctx, client.ObjectKey{Name: req.Name}, &d)
	if apierrors.IsNotFound(err) {
		return allow()
	}
	if err != nil {
		return deny(http.StatusInternalServerError, "read solas Device %s: %v", req.Name, err)
	}
	if d.Annotations[AnnotationPivotedFrom] != p.Source.Ref(req.Namespace, req.Name) {
		// The Device is not the copy of this object.
		return allow()
	}
	if p.DryRun {
		return allow()
	}
	//= spec/solas.md#9-4-webhook
	//# On a delete, the webhook MUST delete the Device.
	err = p.Solas.Delete(ctx, &d, client.Preconditions{UID: &d.UID, ResourceVersion: &d.ResourceVersion})
	switch {
	case err == nil, apierrors.IsNotFound(err):
		return allow()
	case apierrors.IsConflict(err):
		//= spec/solas.md#9-4-webhook
		//# If solas rejects the delete of the Device, the webhook MUST deny the
		//# delete of the old object.
		return deny(http.StatusConflict, "solas refused to delete Device %s: %v", req.Name, err)
	default:
		return deny(http.StatusInternalServerError, "delete solas Device %s: %v", req.Name, err)
	}
}

// patchAnnotation returns a JSON patch that sets pivoted-to on o.
func patchAnnotation(o *unstructured.Unstructured) *admissionv1.AdmissionResponse {
	want := PivotedTo(o.GetName())
	if o.GetAnnotations()[AnnotationPivotedTo] == want {
		return allow()
	}
	var op map[string]any
	if o.GetAnnotations() == nil {
		op = map[string]any{"op": "add", "path": "/metadata/annotations",
			"value": map[string]string{AnnotationPivotedTo: want}}
	} else {
		op = map[string]any{"op": "add", "path": "/metadata/annotations/" + escape(AnnotationPivotedTo), "value": want}
	}
	patch, _ := json.Marshal([]any{op})
	pt := admissionv1.PatchTypeJSONPatch
	return &admissionv1.AdmissionResponse{Allowed: true, Patch: patch, PatchType: &pt}
}

// escape escapes a JSON pointer token, RFC 6901.
func escape(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}

func allow() *admissionv1.AdmissionResponse { return &admissionv1.AdmissionResponse{Allowed: true} }

func deny(code int32, format string, args ...any) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: false,
		Result:  &metav1.Status{Code: code, Message: fmt.Sprintf(format, args...)},
	}
}
