// Package ddbtest connects tests to the dynamodb-local that
// scripts/test.sh starts.
package ddbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// EndpointEnv names the variable that holds the dynamodb-local URL.
const EndpointEnv = "SOLAS_DYNAMODB_ENDPOINT"

// Client returns a client for dynamodb-local. It skips the test when
// EndpointEnv is not set.
func Client(t testing.TB) *dynamodb.Client {
	t.Helper()
	endpoint := os.Getenv(EndpointEnv)
	if endpoint == "" {
		t.Skipf("%s is not set; run the tests with devbox run test", EndpointEnv)
	}
	return dynamodb.New(dynamodb.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider("local", "local", ""),
	})
}

var unsafe = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

// TableName returns a table name that no other test uses.
func TableName(t testing.TB) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	name := unsafe.ReplaceAllString(t.Name(), "-")
	if len(name) > 200 {
		name = name[:200]
	}
	return name + "-" + hex.EncodeToString(b)
}

// DeleteTable removes the table when the test ends.
func DeleteTable(t testing.TB, c *dynamodb.Client, table string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = c.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{
			TableName: aws.String(table),
		})
	})
}
