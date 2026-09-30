// Command solas-pivot moves Devices from a CRD in a cluster's etcd into
// solas, spec section 9 and ADR 0010.
//
//	solas-pivot copy    --from inventory.example.com/v1/devices [--dry-run]
//	solas-pivot verify  --from inventory.example.com/v1/devices
//	solas-pivot webhook --from inventory.example.com/v1/devices --tls-cert c --tls-key k
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/phoban01/solas/pkg/controller/scheme"
	"github.com/phoban01/solas/pkg/pivot"
)

const usage = `usage: solas-pivot copy|verify|webhook [flags]

  copy     create or update a solas Device for each object of the old CRD
  verify   check that each old object has a matching solas Device
  webhook  serve the mutating admission webhook on the old CRD

Run solas-pivot <command> -h for the flags of a command.`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "solas-pivot:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	kubeconfig := fs.String("kubeconfig", os.Getenv("KUBECONFIG"), "kubeconfig; empty means in-cluster")
	from := fs.String("from", "", "old resource as group/version/resource, for example inventory.example.com/v1/devices")
	descPath := fs.String("description-path", "spec.description", "field of the old object that holds the description")
	dryRun := fs.Bool("dry-run", false, "copy: report, and write nothing")
	listen := fs.String("listen", ":8443", "webhook: address to serve on")
	cert := fs.String("tls-cert", "", "webhook: TLS certificate file")
	key := fs.String("tls-key", "", "webhook: TLS key file")
	_ = fs.Parse(args)

	src, err := pivot.ParseSource(*from, *descPath)
	if err != nil {
		return err
	}
	p, err := newPivot(*kubeconfig, src)
	if err != nil {
		return err
	}
	switch cmd {
	case "copy":
		p.DryRun = *dryRun
		return runCopy(ctx, p)
	case "verify":
		return runVerify(ctx, p)
	case "webhook":
		return runWebhook(ctx, p, *listen, *cert, *key)
	}
	return fmt.Errorf("unknown command %q\n%s", cmd, usage)
}

func newPivot(kubeconfig string, src pivot.Source) (*pivot.Pivot, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme.New()})
	if err != nil {
		return nil, err
	}
	return &pivot.Pivot{Old: dyn, Solas: c, Source: src}, nil
}

func runCopy(ctx context.Context, p *pivot.Pivot) error {
	r, err := p.Copy(ctx)
	prefix := ""
	if p.DryRun {
		prefix = "dry run: "
	}
	fmt.Printf("%screated %d, updated %d, unchanged %d, conflicts %d\n",
		prefix, len(r.Created), len(r.Updated), len(r.Unchanged), len(r.Conflicts))
	for _, name := range r.Conflicts {
		fmt.Printf("conflict: solas Device %s exists and is not pivoted from this object\n", name)
	}
	if err != nil {
		return err
	}
	if len(r.Conflicts) > 0 {
		return errors.New("some objects were not copied")
	}
	return nil
}

func runVerify(ctx context.Context, p *pivot.Pivot) error {
	problems, err := p.Verify(ctx)
	if err != nil {
		return err
	}
	for _, msg := range problems {
		fmt.Println(msg)
	}
	//= spec/solas.md#9-3-verify
	//# It MUST exit with a status other than 0 when any check fails.
	if len(problems) > 0 {
		return fmt.Errorf("%d objects do not match their solas Devices", len(problems))
	}
	fmt.Println("every old object has a matching solas Device")
	return nil
}

func runWebhook(ctx context.Context, p *pivot.Pivot, listen, cert, key string) error {
	if cert == "" || key == "" {
		return errors.New("webhook needs --tls-cert and --tls-key")
	}
	mux := http.NewServeMux()
	mux.Handle("POST /mutate", &pivot.Webhook{Pivot: p})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	srv := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	fmt.Printf("pivot webhook for %s on %s\n", p.Source.Resource, listen)
	if err := srv.ListenAndServeTLS(cert, key); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
