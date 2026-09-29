package dynamo

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/phoban01/solas/internal/ddbtest"
)

func TestEnsureTable(t *testing.T) {
	ctx := context.Background()
	c := ddbtest.Client(t)
	table := ddbtest.TableName(t)
	ddbtest.DeleteTable(t, c, table)

	for i := range 2 {
		if err := EnsureTable(ctx, c, table); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}

	d, err := c.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(table)})
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]types.KeyType{}
	for _, k := range d.Table.KeySchema {
		keys[aws.ToString(k.AttributeName)] = k.KeyType
	}
	if keys[attrPK] != types.KeyTypeHash || keys[attrSK] != types.KeyTypeRange {
		t.Errorf("key schema = %v, want pk HASH and sk RANGE", keys)
	}

	ttl, err := c.DescribeTimeToLive(ctx, &dynamodb.DescribeTimeToLiveInput{TableName: aws.String(table)})
	if err != nil {
		t.Fatal(err)
	}
	if got := aws.ToString(ttl.TimeToLiveDescription.AttributeName); got != attrExpires {
		t.Errorf("TTL attribute = %q, want %q", got, attrExpires)
	}
}
