// Package storage reads the storage URL, which picks the shared store of
// the API server, spec 2.6.
package storage

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/phoban01/solas/pkg/storage/dynamo"
)

// Kind names a store.
type Kind string

const (
	// DynamoDB is the DynamoDB store, spec 2.1 to 2.4.
	DynamoDB Kind = "dynamodb"
	// Etcd is the etcd store, spec 2.7.
	Etcd Kind = "etcd"
)

// Config is a parsed storage URL.
type Config struct {
	Kind Kind

	// Table, Region, Endpoint, and CreateTable hold the DynamoDB settings.
	// An empty Region or Endpoint means the AWS default.
	Table       string
	Region      string
	Endpoint    string
	CreateTable bool

	// Endpoints holds the etcd client URLs, for example
	// http://10.0.0.1:2379.
	Endpoints []string
}

//= spec/solas.md#2-6-storage-url
//# The store MUST be a setting of the API server, in the form of one
//# storage URL.

// Parse reads a storage URL.
func Parse(raw string) (Config, error) {
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return Config{}, fmt.Errorf("storage URL %q: want <scheme>://...", raw)
	}
	authority, query, _ := strings.Cut(rest, "?")
	authority, path, _ := strings.Cut(authority, "/")
	if path != "" {
		return Config{}, fmt.Errorf("storage URL %q: a path is not allowed", raw)
	}
	if strings.Contains(authority, "@") {
		return Config{}, fmt.Errorf("storage URL %q: credentials come from the environment, not the URL", raw)
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return Config{}, fmt.Errorf("storage URL %q: %w", raw, err)
	}

	switch Kind(scheme) {
	case DynamoDB:
		return parseDynamo(raw, authority, values)
	case Etcd:
		return parseEtcd(raw, authority, values)
	}
	//= spec/solas.md#2-6-storage-url
	//# The server MUST reject a storage URL with another scheme.
	return Config{}, fmt.Errorf("storage URL %q: scheme %q is not dynamodb or etcd", raw, scheme)
}

//= spec/solas.md#2-6-storage-url
//# A URL with the scheme `dynamodb` MUST select the DynamoDB store.

func parseDynamo(raw, table string, values url.Values) (Config, error) {
	//= spec/solas.md#2-6-storage-url
	//# The server MUST reject a storage URL with a query key that this section
	//# does not name for its scheme.
	if err := onlyKeys(raw, values, "region", "endpoint", "create-table"); err != nil {
		return Config{}, err
	}
	//= spec/solas.md#2-6-storage-url
	//# An empty table name selects the default table name.
	if table == "" {
		table = dynamo.DefaultTable
	}
	c := Config{Kind: DynamoDB, Table: table, Region: values.Get("region"), Endpoint: values.Get("endpoint")}
	if v := values.Get("create-table"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("storage URL %q: create-table=%q is not true or false", raw, v)
		}
		c.CreateTable = b
	}
	if c.Endpoint != "" {
		if u, err := url.Parse(c.Endpoint); err != nil || u.Scheme == "" || u.Host == "" {
			return Config{}, fmt.Errorf("storage URL %q: endpoint %q is not a URL", raw, c.Endpoint)
		}
	}
	return c, nil
}

//= spec/solas.md#2-6-storage-url
//# A URL with the scheme `etcd` MUST select the etcd store.

func parseEtcd(raw, hosts string, values url.Values) (Config, error) {
	if err := onlyKeys(raw, values); err != nil {
		return Config{}, err
	}
	if hosts == "" {
		return Config{}, fmt.Errorf("storage URL %q: no etcd endpoints", raw)
	}
	c := Config{Kind: Etcd}
	for _, h := range strings.Split(hosts, ",") {
		host, port, err := net.SplitHostPort(h)
		if err != nil || host == "" {
			return Config{}, fmt.Errorf("storage URL %q: endpoint %q is not host:port", raw, h)
		}
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return Config{}, fmt.Errorf("storage URL %q: endpoint %q has a bad port", raw, h)
		}
		c.Endpoints = append(c.Endpoints, "http://"+h)
	}
	return c, nil
}

func onlyKeys(raw string, values url.Values, allowed ...string) error {
	for k := range values {
		ok := false
		for _, a := range allowed {
			ok = ok || k == a
		}
		if !ok {
			return fmt.Errorf("storage URL %q: unknown query key %q", raw, k)
		}
	}
	return nil
}

// String prints the URL of the configuration.
func (c Config) String() string {
	switch c.Kind {
	case DynamoDB:
		q := url.Values{}
		if c.Region != "" {
			q.Set("region", c.Region)
		}
		if c.Endpoint != "" {
			q.Set("endpoint", c.Endpoint)
		}
		if c.CreateTable {
			q.Set("create-table", "true")
		}
		s := "dynamodb://" + c.Table
		if len(q) > 0 {
			s += "?" + q.Encode()
		}
		return s
	case Etcd:
		hosts := make([]string, len(c.Endpoints))
		for i, e := range c.Endpoints {
			hosts[i] = strings.TrimPrefix(e, "http://")
		}
		return "etcd://" + strings.Join(hosts, ",")
	}
	return ""
}
