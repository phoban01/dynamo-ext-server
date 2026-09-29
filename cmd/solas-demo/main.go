// Command solas-demo holds the two demo parts that are not solas: a device
// gatekeeper that checks fencing tokens, and a workload that uses a device
// with the token of its claim, spec 6.6.
//
//	solas-demo device   --listen :9000
//	solas-demo workload --namespace ns --claim name --cluster-id a --device http://host:9000
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/phoban01/solas/pkg/controller/scheme"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: solas-demo device|workload [flags]")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch os.Args[1] {
	case "device":
		err = runDevice(ctx, os.Args[2:])
	case "workload":
		err = runWorkload(ctx, os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runDevice(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("device", flag.ExitOnError)
	listen := fs.String("listen", ":9000", "address to listen on")
	_ = fs.Parse(args)
	d := &device{now: time.Now}
	srv := &http.Server{Addr: *listen, Handler: d.handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	fmt.Printf("device gatekeeper listening on %s\n", *listen)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func runWorkload(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("workload", flag.ExitOnError)
	ns := fs.String("namespace", "default", "namespace of the claim")
	name := fs.String("claim", "", "name of the claim")
	clusterID := fs.String("cluster-id", "", "ID of this member cluster")
	dev := fs.String("device", "", "URL of the device gatekeeper")
	refresh := fs.Duration("refresh", 5*time.Second, "how often to read the claim")
	every := fs.Duration("every", time.Second, "how often to use the device")
	_ = fs.Parse(args)
	if *name == "" || *dev == "" || *clusterID == "" {
		return fmt.Errorf("workload needs --claim, --cluster-id, and --device")
	}
	c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme.New()})
	if err != nil {
		return err
	}
	w := &workload{
		client: c, key: client.ObjectKey{Namespace: *ns, Name: *name},
		clusterID: *clusterID, device: *dev, refresh: *refresh, every: *every,
	}
	return w.run(ctx)
}
