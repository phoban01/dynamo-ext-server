package device

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/phoban01/solas/pkg/apis/solas"
)

// tableConvertor prints the columns of kubectl get devices, spec 10.2.
type tableConvertor struct{}

var columns = []metav1.TableColumnDefinition{
	{Name: "Name", Type: "string", Format: "name"},
	{Name: "Holder", Type: "string", Description: "member/namespace/claim"},
	{Name: "Priority", Type: "integer"},
	{Name: "Token", Type: "integer", Description: "fencing token of the last bind"},
	{Name: "Preemptible", Type: "boolean"},
	{Name: "Preemption", Type: "string", Description: "the claim that asks, and its priority"},
	{Name: "Ready", Type: "string"},
	{Name: "Age", Type: "string"},
}

func (tableConvertor) ConvertToTable(_ context.Context, obj runtime.Object, _ runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{ColumnDefinitions: columns}
	var devices []solas.Device
	switch o := obj.(type) {
	case *solas.Device:
		devices = []solas.Device{*o}
	case *solas.DeviceList:
		devices = o.Items
		table.ResourceVersion, table.Continue, table.RemainingItemCount = o.ResourceVersion, o.Continue, o.RemainingItemCount
	default:
		return nil, fmt.Errorf("unexpected object %T", obj)
	}
	for i := range devices {
		d := &devices[i]
		holder, prio := "<free>", ""
		if r := d.Status.ClaimRef; r != nil {
			holder = r.Member + "/" + r.Namespace + "/" + r.Name
			prio = strconv.Itoa(int(r.Priority))
		}
		preemption := ""
		if p := d.Status.Preemption; p != nil {
			preemption = fmt.Sprintf("%s/%s/%s@%d", p.Claim.Member, p.Claim.Namespace, p.Claim.Name, p.Claim.Priority)
		}
		ready := "Unknown"
		if c := meta.FindStatusCondition(d.Status.Conditions, "Ready"); c != nil {
			ready = string(c.Status)
		}
		table.Rows = append(table.Rows, metav1.TableRow{
			Cells: []any{d.Name, holder, prio, d.Status.FencingToken, d.Spec.Preemptible, preemption, ready,
				duration.HumanDuration(metav1.Now().Sub(d.CreationTimestamp.Time))},
			Object: runtime.RawExtension{Object: d},
		})
	}
	return table, nil
}
