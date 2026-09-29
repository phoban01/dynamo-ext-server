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
	"github.com/phoban01/solas/pkg/storage/dynamo"
)

// Options holds the flags of the solas API server.
type Options struct {
	RecommendedOptions *genericoptions.RecommendedOptions
	Registry           basecompatibility.ComponentGlobalsRegistry

	DynamoEndpoint    string
	DynamoRegion      string
	DynamoTable       string
	DynamoCreateTable bool
	EventRetention    time.Duration
	PollInterval      time.Duration
	// Prefix separates the data of different API servers in one table.
	// All member clusters must use the same prefix to share state.
	Prefix string
}

// NewOptions returns the default options.
func NewOptions() *Options {
	o := &Options{
		RecommendedOptions: genericoptions.NewRecommendedOptions("/registry", apiserver.StorageCodec()),
		Registry:           compatibility.DefaultComponentGlobalsRegistry,
		DynamoRegion:       "us-east-1",
		DynamoTable:        dynamo.DefaultTable,
		EventRetention:     dynamo.DefaultEventRetention,
		PollInterval:       dynamo.DefaultPollInterval,
		Prefix:             "/registry",
	}
	// The server stores its data in DynamoDB, not etcd.
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
	flags.StringVar(&o.DynamoTable, "dynamodb-table", o.DynamoTable, "DynamoDB table. All member clusters must use the same table.")
	//= spec/solas.md#2-1-table
	//# The table MUST be in one AWS region.
	flags.StringVar(&o.DynamoRegion, "dynamodb-region", o.DynamoRegion, "AWS region of the table.")
	flags.StringVar(&o.DynamoEndpoint, "dynamodb-endpoint", o.DynamoEndpoint, "DynamoDB endpoint URL, for example http://dynamodb-local:8000. Empty means the AWS endpoint of the region.")
	flags.BoolVar(&o.DynamoCreateTable, "dynamodb-create-table", o.DynamoCreateTable, "Create the table at start when it does not exist.")
	flags.DurationVar(&o.EventRetention, "event-retention", o.EventRetention, "How long events stay in the event log.")
	flags.DurationVar(&o.PollInterval, "poll-interval", o.PollInterval, "How often a watch polls the event log.")
	flags.StringVar(&o.Prefix, "storage-prefix", o.Prefix, "Key prefix in the table. All member clusters must use the same prefix.")

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
	if o.DynamoTable == "" {
		errs = append(errs, fmt.Errorf("--dynamodb-table must not be empty"))
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

	client, err := o.dynamoClient(ctx)
	if err != nil {
		return nil, err
	}
	if o.DynamoCreateTable {
		if err := dynamo.EnsureTable(ctx, client, o.DynamoTable); err != nil {
			return nil, err
		}
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
	c.RESTOptionsGetter = apiserver.RESTOptionsGetter{
		Dynamo: dynamo.Config{
			Client:         client,
			Table:          o.DynamoTable,
			EventRetention: o.EventRetention,
			PollInterval:   o.PollInterval,
		},
		Codec:           apiserver.StorageCodec(),
		EncodeVersioner: apiserver.StorageVersioner(),
		Prefix:          o.Prefix,
	}
	return &apiserver.Config{GenericConfig: c}, nil
}

func (o *Options) dynamoClient(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(o.DynamoRegion))
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	return dynamodb.NewFromConfig(cfg, func(opts *dynamodb.Options) {
		if o.DynamoEndpoint != "" {
			opts.BaseEndpoint = aws.String(o.DynamoEndpoint)
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
