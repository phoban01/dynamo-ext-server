package controller

import (
	"context"

	"k8s.io/client-go/rest"
	"k8s.io/utils/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/phoban01/solas/pkg/controller/claim"
	"github.com/phoban01/solas/pkg/controller/member"
	"github.com/phoban01/solas/pkg/controller/scheme"
	"github.com/phoban01/solas/pkg/controller/sweeper"
)

// Run starts the controller and blocks until ctx ends.
func Run(ctx context.Context, cfg *rest.Config, o *Options) error {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme.New(),
		Metrics:                metricsserver.Options{BindAddress: o.MetricsAddr},
		HealthProbeBindAddress: o.ProbeAddr,
		//= spec/solas.md#6-2-controller
		//# The controller MUST use a leader election lease in its own cluster.

		//= spec/solas.md#6-2-controller
		//# Each member cluster MUST run at most one active claim controller.
		LeaderElection:          o.LeaderElection,
		LeaderElectionID:        "solas-controller",
		LeaderElectionNamespace: o.LeaderElectionNS,
	})
	if err != nil {
		return err
	}

	clk := clock.RealClock{}
	// Devices and Members are read with no cache, so every list is
	// consistent, spec 6.3 and 8.3.
	members := &member.Manager{
		Client:    mgr.GetClient(),
		Reader:    mgr.GetAPIReader(),
		ClusterID: o.ClusterID,
		Lease:     o.LeaseDuration,
		Margin:    o.LeaseMargin,
		Clock:     clk,
	}
	claims := &claim.Reconciler{
		Client:    mgr.GetClient(),
		Reader:    mgr.GetAPIReader(),
		ClusterID: o.ClusterID,
		Member:    members,
		Resync:    o.LeaseDuration / 3,
	}
	members.Drained = claims.Drained
	if err := claims.SetupWithManager(mgr); err != nil {
		return err
	}
	for _, r := range []interface {
		Start(context.Context) error
	}{
		members,
		&claim.Orphans{Reconciler: claims, Interval: o.SweepInterval},
		&sweeper.Sweeper{
			Client:    mgr.GetClient(),
			Reader:    mgr.GetAPIReader(),
			ClusterID: o.ClusterID,
			Clock:     clk,
			Interval:  o.SweepInterval,
		},
	} {
		if err := mgr.Add(runnable{r}); err != nil {
			return err
		}
	}
	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		return err
	}
	return mgr.Start(ctx)
}

// runnable runs only on the leader, like the reconciler.
type runnable struct {
	r interface{ Start(context.Context) error }
}

func (r runnable) Start(ctx context.Context) error { return r.r.Start(ctx) }
func (runnable) NeedLeaderElection() bool          { return true }
