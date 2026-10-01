package storage

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// defaultRegion is the AWS region when neither the storage URL nor the
// environment names one.
const defaultRegion = "us-east-1"

// DynamoClient returns a DynamoDB client for a DynamoDB storage URL. The
// credentials come from the AWS environment.
func DynamoClient(ctx context.Context, c Config) (*dynamodb.Client, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if c.Region != "" {
		opts = append(opts, awsconfig.WithRegion(c.Region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	if cfg.Region == "" {
		cfg.Region = defaultRegion
	}
	return dynamodb.NewFromConfig(cfg, func(opts *dynamodb.Options) {
		if c.Endpoint != "" {
			opts.BaseEndpoint = aws.String(c.Endpoint)
		}
	}), nil
}
