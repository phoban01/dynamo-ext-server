// Command solas serves the solas.dev API from DynamoDB and runs the
// member controller of one member cluster, ADR 0012.
package main

import (
	"os"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/cli"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/phoban01/solas/pkg/cmd/solas"
)

func main() {
	// Both parts log through klog, so the flags -v and --logging-format
	// set the logs of both.
	ctrl.SetLogger(klog.NewKlogr())
	ctx := genericapiserver.SetupSignalContext()
	cmd := solas.NewCommand(ctx, solas.NewOptions())
	os.Exit(cli.Run(cmd))
}
