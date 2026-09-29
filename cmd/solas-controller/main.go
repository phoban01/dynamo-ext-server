// Command solas-controller runs the member manager, the claim reconciler,
// and the sweeper of one member cluster.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/pflag"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/phoban01/solas/pkg/controller"
)

func main() {
	o := controller.NewOptions()
	fs := pflag.NewFlagSet("solas-controller", pflag.ExitOnError)
	o.AddFlags(fs)
	zapOpts := zap.Options{}
	gofs := flag.NewFlagSet("zap", flag.ExitOnError)
	zapOpts.BindFlags(gofs)
	fs.AddGoFlagSet(gofs)
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if err := o.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
	if err := controller.Run(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie(), o); err != nil {
		ctrl.Log.Error(err, "controller stopped")
		os.Exit(1)
	}
}
