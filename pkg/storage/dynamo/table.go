package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Attribute names, spec 2.1 and 2.2.
const (
	attrPK      = "pk"
	attrSK      = "sk"
	attrRV      = "rv"
	attrN       = "n"
	attrValue   = "value"
	attrPrev    = "prev"
	attrType    = "type"
	attrKey     = "key"
	attrExpires = "expires"
)

// TableAPI is the part of the DynamoDB client that EnsureTable uses.
type TableAPI interface {
	CreateTable(context.Context, *dynamodb.CreateTableInput, ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
	DescribeTable(context.Context, *dynamodb.DescribeTableInput, ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
	DescribeTimeToLive(context.Context, *dynamodb.DescribeTimeToLiveInput, ...func(*dynamodb.Options)) (*dynamodb.DescribeTimeToLiveOutput, error)
	UpdateTimeToLive(context.Context, *dynamodb.UpdateTimeToLiveInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateTimeToLiveOutput, error)
}

// EnsureTable creates the table when it does not exist, waits until it is
// active, and enables TTL. It is safe to call again on an existing table.
func EnsureTable(ctx context.Context, c TableAPI, table string) error {
	//= spec/solas.md#2-1-table
	//# The table MUST have a string partition key named `pk` and a string sort
	//# key named `sk`.
	_, err := c.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(table),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String(attrPK), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String(attrSK), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String(attrPK), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String(attrSK), KeyType: types.KeyTypeRange},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	var inUse *types.ResourceInUseException
	if err != nil && !errors.As(err, &inUse) {
		return fmt.Errorf("create table %s: %w", table, err)
	}

	w := dynamodb.NewTableExistsWaiter(c)
	if err := w.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(table)}, 2*time.Minute); err != nil {
		return fmt.Errorf("wait for table %s: %w", table, err)
	}

	ttl, err := c.DescribeTimeToLive(ctx, &dynamodb.DescribeTimeToLiveInput{TableName: aws.String(table)})
	if err != nil {
		return fmt.Errorf("describe TTL of table %s: %w", table, err)
	}
	if d := ttl.TimeToLiveDescription; d != nil && d.TimeToLiveStatus == types.TimeToLiveStatusEnabled {
		return nil
	}
	//= spec/solas.md#2-1-table
	//# The table MUST enable TTL on the attribute `expires`.
	_, err = c.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(table),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String(attrExpires),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		return fmt.Errorf("enable TTL on table %s: %w", table, err)
	}
	return nil
}
