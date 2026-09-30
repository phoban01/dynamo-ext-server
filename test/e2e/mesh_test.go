//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	claimsv1alpha1 "github.com/phoban01/solas/pkg/apis/claims/v1alpha1"
	solasv1alpha1 "github.com/phoban01/solas/pkg/apis/solas/v1alpha1"
)

func stringsReader(s string) *strings.Reader { return strings.NewReader(s) }

// eventually polls cond once a second until it holds or the timeout ends.
func eventually(t *testing.T, timeout time.Duration, what string, cond func(ctx context.Context) bool) {
	t.Helper()
	err := wait.PollUntilContextTimeout(context.Background(), time.Second, timeout, true,
		func(ctx context.Context) (bool, error) { return cond(ctx), nil })
	if err != nil {
		t.Fatalf("%s: not true after %v", what, timeout)
	}
}

func phase(ctx context.Context, c client.Client) claimsv1alpha1.ClaimPhase {
	var claim claimsv1alpha1.DeviceClaim
	if err := c.Get(ctx, client.ObjectKey{Namespace: "work", Name: "job"}, &claim); err != nil {
		return ""
	}
	return claim.Status.Phase
}

func holder(ctx context.Context, c client.Client) string {
	var d solasv1alpha1.Device
	if err := c.Get(ctx, client.ObjectKey{Name: "gpu-1"}, &d); err != nil || d.Status.ClaimRef == nil {
		return ""
	}
	return fmt.Sprintf("%s/%d", d.Status.ClaimRef.Member, d.Status.FencingToken)
}

type use struct {
	Claim    string `json:"claim"`
	Token    int64  `json:"token"`
	Accepted bool   `json:"accepted"`
}

func deviceLog(url string) []use {
	resp, err := http.Get(url + "/log")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var log []use
	_ = json.NewDecoder(resp.Body).Decode(&log)
	return log
}

func seen(url, cluster string, token int64, accepted bool) bool {
	for _, u := range deviceLog(url) {
		if u.Claim == cluster+"/work/job" && u.Token == token && u.Accepted == accepted {
			return true
		}
	}
	return false
}

func memberUID(ctx context.Context, c client.Client, name string) string {
	var m solasv1alpha1.Member
	if err := c.Get(ctx, client.ObjectKey{Name: name}, &m); err != nil {
		return ""
	}
	return string(m.UID)
}

// TestMesh runs the demo scenario: sharing, a race, fencing, and rejoin.
// Spec sections 5 to 8.
func TestMesh(t *testing.T) {
	var winner, loser, deviceURL, oldUID string
	clients := map[string]client.Client{}

	f := features.New("two clusters share one table").
		Setup(func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			clients["a"], clients["b"] = clientFor(t, "a"), clientFor(t, "b")
			return ctx
		}).
		Assess("a device created in a appears in b", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			d := &solasv1alpha1.Device{ObjectMeta: metav1.ObjectMeta{Name: "gpu-1", Labels: map[string]string{"kind": "gpu"}}}
			if err := clients["a"].Create(ctx, d); err != nil {
				t.Fatal(err)
			}
			eventually(t, 30*time.Second, "gpu-1 visible in b", func(ctx context.Context) bool {
				var got solasv1alpha1.Device
				return clients["b"].Get(ctx, client.ObjectKey{Name: "gpu-1"}, &got) == nil
			})
			return ctx
		}).
		Assess("claims in a and b race and one wins", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			run(t, "", "docker", "rm", "-f", "solas-device-gpu-1")
			run(t, "", "docker", "run", "-d", "--name", "solas-device-gpu-1", "--network", "solas-mesh", "solas-demo:dev", "device")
			ip := strings.TrimSpace(run(t, "", "docker", "inspect", "-f",
				`{{(index .NetworkSettings.Networks "solas-mesh").IPAddress}}`, "solas-device-gpu-1"))
			deviceURL = "http://" + ip + ":9000"
			eventually(t, 30*time.Second, "gatekeeper up", func(context.Context) bool {
				_, err := http.Get(deviceURL + "/log")
				return err == nil
			})
			manifest, err := os.ReadFile(root + "/demo/k3d/manifests/claim.yaml")
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"a", "b"} {
				m := strings.NewReplacer("CLUSTER_ID", name, "DEVICE_URL", deviceURL).Replace(string(manifest))
				run(t, m, "kubectl", "--kubeconfig", kubeconfig(name), "apply", "-f", "-")
			}
			eventually(t, 60*time.Second, "one claim Bound", func(ctx context.Context) bool {
				return (phase(ctx, clients["a"]) == claimsv1alpha1.ClaimBound) != (phase(ctx, clients["b"]) == claimsv1alpha1.ClaimBound)
			})
			winner, loser = "a", "b"
			if phase(ctx, clients["b"]) == claimsv1alpha1.ClaimBound {
				winner, loser = "b", "a"
			}
			if h := holder(ctx, clients[loser]); h != winner+"/1" {
				t.Fatalf("gpu-1 holder = %s, want %s/1", h, winner)
			}
			time.Sleep(3 * time.Second)
			if p := phase(ctx, clients[loser]); p != claimsv1alpha1.ClaimPending {
				t.Fatalf("loser claim phase = %s, want Pending", p)
			}
			eventually(t, 30*time.Second, "winner uses the device", func(context.Context) bool {
				return seen(deviceURL, winner, 1, true)
			})
			return ctx
		}).
		Assess("the device rejects the stale holder after a reclaim", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			oldUID = memberUID(ctx, clients[winner], winner)
			node := "k3d-e2e-" + winner + "-server-0"
			run(t, "", "docker", "pause", node)
			eventually(t, 90*time.Second, "the loser's claim becomes Bound", func(ctx context.Context) bool {
				return phase(ctx, clients[loser]) == claimsv1alpha1.ClaimBound
			})
			if h := holder(ctx, clients[loser]); h != loser+"/2" {
				t.Fatalf("gpu-1 holder = %s, want %s/2", h, loser)
			}
			eventually(t, 30*time.Second, "the new holder uses the device", func(context.Context) bool {
				return seen(deviceURL, loser, 2, true)
			})
			run(t, "", "docker", "unpause", node)
			eventually(t, 60*time.Second, "the device rejects token 1", func(context.Context) bool {
				return seen(deviceURL, winner, 1, false)
			})
			return ctx
		}).
		Assess("the old holder joins again and its claim is Lost", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			eventually(t, 90*time.Second, "new member UID", func(ctx context.Context) bool {
				uid := memberUID(ctx, clients[winner], winner)
				return uid != "" && uid != oldUID
			})
			eventually(t, 60*time.Second, "claim Lost", func(ctx context.Context) bool {
				return phase(ctx, clients[winner]) == claimsv1alpha1.ClaimLost
			})
			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
