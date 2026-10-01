package storage

import (
	"reflect"
	"testing"
)

//= spec/solas.md#2-6-storage-url
//= type=test
//# A URL with the scheme `dynamodb` MUST select the DynamoDB store.

//= spec/solas.md#2-6-storage-url
//= type=test
//# A URL with the scheme `etcd` MUST select the etcd store.

//= spec/solas.md#2-6-storage-url
//= type=test
//# The server MUST reject a storage URL with another scheme.

//= spec/solas.md#2-6-storage-url
//= type=test
//# The server MUST reject a storage URL with a query key that this section
//# does not name for its scheme.

func TestParseURL(t *testing.T) {
	ok := []struct {
		url  string
		want Config
	}{
		{"dynamodb://solas", Config{Kind: DynamoDB, Table: "solas"}},
		{"dynamodb://", Config{Kind: DynamoDB, Table: "solas"}},
		{"dynamodb://devices?region=eu-west-1", Config{Kind: DynamoDB, Table: "devices", Region: "eu-west-1"}},
		{"dynamodb://solas?endpoint=http://172.18.0.2:8000&create-table=true",
			Config{Kind: DynamoDB, Table: "solas", Endpoint: "http://172.18.0.2:8000", CreateTable: true}},
		{"dynamodb://solas/", Config{Kind: DynamoDB, Table: "solas"}},
		{"etcd://127.0.0.1:2379", Config{Kind: Etcd, Endpoints: []string{"http://127.0.0.1:2379"}}},
		{"etcd://etcd-0:2379,etcd-1:2379", Config{Kind: Etcd, Endpoints: []string{"http://etcd-0:2379", "http://etcd-1:2379"}}},
	}
	for _, c := range ok {
		got, err := Parse(c.url)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.url, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q) = %+v, want %+v", c.url, got, c.want)
		}
		again, err := Parse(got.String())
		if err != nil || !reflect.DeepEqual(again, got) {
			t.Errorf("Parse(%q).String() = %q does not parse back: %+v, %v", c.url, got.String(), again, err)
		}
	}

	bad := []string{
		"",
		"solas",
		"s3://bucket",
		"dynamodb://solas?table=x",
		"dynamodb://solas?create-table=maybe",
		"dynamodb://solas?endpoint=not-a-url",
		"dynamodb://solas/extra",
		"dynamodb://key:secret@solas",
		"etcd://",
		"etcd://127.0.0.1",
		"etcd://127.0.0.1:0",
		"etcd://127.0.0.1:2379?region=x",
		"etcd://:2379",
	}
	for _, u := range bad {
		if c, err := Parse(u); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", u, c)
		}
	}
}

func TestURLString(t *testing.T) {
	c := Config{Kind: DynamoDB, Table: "solas", Endpoint: "http://172.18.0.2:8000", CreateTable: true}
	if got, want := c.String(), "dynamodb://solas?create-table=true&endpoint=http://172.18.0.2:8000"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	odd := Config{Kind: DynamoDB, Table: "solas", Endpoint: "http://h:8000/a&b"}
	if back, err := Parse(odd.String()); err != nil || back.Endpoint != odd.Endpoint {
		t.Errorf("Parse(%q) = %+v, %v; want the endpoint back", odd.String(), back, err)
	}
}
