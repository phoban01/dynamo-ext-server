// Command solas-apiserver serves the solas.dev API group from DynamoDB.
package main

import (
	"os"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/cli"

	"github.com/phoban01/solas/pkg/cmd/server"
)

func main() {
	ctx := genericapiserver.SetupSignalContext()
	cmd := server.NewCommand(ctx, server.NewOptions())
	os.Exit(cli.Run(cmd))
}
