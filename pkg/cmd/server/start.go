// Package server holds the command of the solas API server.
package server

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/spf13/cobra"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/util/compatibility"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	basecompatibility "k8s.io/component-base/compatibility"
	baseversion "k8s.io/component-base/version"
	netutils "k8s.io/utils/net"

	"github.com/phoban01/solas/pkg/apiserver"
	generatedopenapi "github.com/phoban01/solas/pkg/generated/openapi"
	storageurl "github.com/phoban01/solas/pkg/storage"
	"github.com/phoban01/solas/pkg/storage/dynamo"
)

// Options holds the flags of the solas API server.
type Options struct {
	RecommendedOptions *genericoptions.RecommendedOptions
	Registry           basecompatibility.ComponentGlobalsRegistry

	// StorageURL picks the shared store, spec 2.6.
	StorageURL     string
	EventRetention time.Duration
	PollInterval   time.Duration
	// Prefix separates the data of different API servers in one table.
	// All member clusters must use the same prefix to share state.
	Prefix string
}

// NewOptions returns the default options.
func NewOptions() *Options {
	o := &Options{
		RecommendedOptions: genericoptions.NewRecommendedOptions("/registry", apiserver.StorageCodec()),
		Registry:           compatibility.DefaultComponentGlobalsRegistry,
		StorageURL:         "dynamodb://" + dynamo.DefaultTable,
		EventRetention:     dynamo.DefaultEventRetention,
		PollInterval:       dynamo.DefaultPollInterval,
		Prefix:             "/registry",
	}
	// The storage URL picks the store, so the server does not take the
	// etcd flags of the generic API server.
	o.RecommendedOptions.Etcd = nil
	return o
}

// NewCommand returns the command that starts the server.
func NewCommand(ctx context.Context, o *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "solas-apiserver",
		Short: "Serve the solas.dev API from DynamoDB",
		PersistentPreRunE: func(*cobra.Command, []string) error {
			return o.Registry.Set()
		},
		RunE: func(c *cobra.Command, _ []string) error {
			if err := o.Validate(); err != nil {
				return err
			}
			return o.Run(c.Context())
		},
	}
	cmd.SetContext(ctx)
	flags := cmd.Flags()
	o.RecommendedOptions.AddFlags(flags)

	//= spec/solas.md#2-1-table
	//# All member clusters MUST use the same table.

	//= spec/solas.md#2-1-table
	//# The table MUST be in one AWS region.

	//= spec/solas.md#2-6-storage-url
	//# All member clusters MUST use the same store.
	flags.StringVar(&o.StorageURL, "storage-url", o.StorageURL,
		"The shared store. All member clusters must use the same store. "+
			"dynamodb://<table>?region=<region>&endpoint=<url>&create-table=true picks a DynamoDB table in one region; "+
			"etcd://<host:port>[,<host:port>...] picks an etcd cluster.")
	flags.DurationVar(&o.EventRetention, "event-retention", o.EventRetention, "How long events stay in the event log of the DynamoDB store.")
	flags.DurationVar(&o.PollInterval, "poll-interval", o.PollInterval, "How often a watch polls the event log of the DynamoDB store.")
	flags.StringVar(&o.Prefix, "storage-prefix", o.Prefix, "Key prefix in the store. All member clusters must use the same prefix.")

	_, _ = o.Registry.ComponentGlobalsOrRegister(basecompatibility.DefaultKubeComponent,
		basecompatibility.NewEffectiveVersionFromString(baseversion.DefaultKubeBinaryVersion, "", ""),
		utilfeature.DefaultMutableFeatureGate)
	o.Registry.AddFlags(flags)
	return cmd
}

// Validate checks the options.
func (o *Options) Validate() error {
	errs := o.RecommendedOptions.Validate()
	errs = append(errs, o.Registry.Validate()...)
	if _, err := storageurl.Parse(o.StorageURL); err != nil {
		errs = append(errs, fmt.Errorf("--storage-url: %w", err))
	}
	if o.PollInterval <= 0 || o.EventRetention <= 0 {
		errs = append(errs, fmt.Errorf("--poll-interval and --event-retention must be above 0"))
	}
	return utilerrors.NewAggregate(errs)
}

// Config builds the server configuration.
func (o *Options) Config(ctx context.Context) (*apiserver.Config, error) {
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts(
		"localhost", nil, []net.IP{netutils.ParseIPSloppy("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("create self-signed certificates: %w", err)
	}

	getter, err := o.restOptionsGetter(ctx)
	if err != nil {
		return nil, err
	}

	c := genericapiserver.NewRecommendedConfig(apiserver.Codecs)
	namer := openapi.NewDefinitionNamer(apiserver.Scheme)
	c.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(generatedopenapi.GetOpenAPIDefinitions, namer)
	c.OpenAPIConfig.Info.Title = "solas"
	c.OpenAPIConfig.Info.Version = "v1alpha1"
	c.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(generatedopenapi.GetOpenAPIDefinitions, namer)
	c.OpenAPIV3Config.Info.Title = "solas"
	c.OpenAPIV3Config.Info.Version = "v1alpha1"
	c.FeatureGate = o.Registry.FeatureGateFor(basecompatibility.DefaultKubeComponent)
	c.EffectiveVersion = o.Registry.EffectiveVersionFor(basecompatibility.DefaultKubeComponent)

	if err := o.RecommendedOptions.ApplyTo(c); err != nil {
		return nil, err
	}
	c.RESTOptionsGetter = getter
	return &apiserver.Config{GenericConfig: c}, nil
}

// restOptionsGetter builds the store that the storage URL picks.
func (o *Options) restOptionsGetter(ctx context.Context) (apiserver.RESTOptionsGetter, error) {
	getter := apiserver.RESTOptionsGetter{
		Codec:           apiserver.StorageCodec(),
		EncodeVersioner: apiserver.StorageVersioner(),
		Prefix:          o.Prefix,
	}
	sc, err := storageurl.Parse(o.StorageURL)
	if err != nil {
		return getter, fmt.Errorf("--storage-url: %w", err)
	}
	if sc.Kind == storageurl.Etcd {
		getter.EtcdServers = sc.Endpoints
		return getter, nil
	}
	client, err := DynamoClient(ctx, sc)
	if err != nil {
		return getter, err
	}
	//= spec/solas.md#2-6-storage-url
	//# The query key `create-table` with the value `true` makes the server
	//# create the table at start when it does not exist.
	if sc.CreateTable {
		if err := dynamo.EnsureTable(ctx, client, sc.Table); err != nil {
			return getter, err
		}
	}
	getter.Dynamo = dynamo.Config{
		Client:         client,
		Table:          sc.Table,
		EventRetention: o.EventRetention,
		PollInterval:   o.PollInterval,
	}
	return getter, nil
}

// defaultRegion is the AWS region when neither the storage URL nor the
// environment names one.
const defaultRegion = "us-east-1"

// DynamoClient returns a DynamoDB client for a DynamoDB storage URL. The
// credentials come from the AWS environment.
func DynamoClient(ctx context.Context, sc storageurl.Config) (*dynamodb.Client, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if sc.Region != "" {
		opts = append(opts, awsconfig.WithRegion(sc.Region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	if cfg.Region == "" {
		cfg.Region = defaultRegion
	}
	return dynamodb.NewFromConfig(cfg, func(opts *dynamodb.Options) {
		if sc.Endpoint != "" {
			opts.BaseEndpoint = aws.String(sc.Endpoint)
		}
	}), nil
}

// Run starts the server and blocks until ctx ends.
func (o *Options) Run(ctx context.Context) error {
	config, err := o.Config(ctx)
	if err != nil {
		return err
	}
	server, err := config.Complete().New()
	if err != nil {
		return err
	}
	return server.GenericAPIServer.PrepareRun().RunWithContext(ctx)
}
