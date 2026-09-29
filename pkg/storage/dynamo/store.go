// Package dynamo implements the Kubernetes storage.Interface on one
// DynamoDB table. spec/solas.md sections 2 to 4 give the rules.
package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
)

// API is the part of the DynamoDB client that the store uses.
type API interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	DescribeTable(context.Context, *dynamodb.DescribeTableInput, ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
}

// Default settings.
const (
	DefaultTable          = "solas"
	DefaultEventRetention = time.Hour
	DefaultPollInterval   = 200 * time.Millisecond
)

// Config holds the settings that all stores of one API server share.
type Config struct {
	Client API
	//= spec/solas.md#2-1-table
	//# The table name MUST be a setting of the API server.
	Table string
	//= spec/solas.md#4-1-event-log
	//# The retention time MUST be a setting of the API server.
	EventRetention time.Duration
	PollInterval   time.Duration
}

type store struct {
	client         API
	table          string
	retention      time.Duration
	pollInterval   time.Duration
	codec          runtime.Codec
	versioner      storage.Versioner
	newFunc        func() runtime.Object
	newListFunc    func() runtime.Object
	resourcePrefix string
	// resource names the counter item and the partitions of this resource.
	resource      string
	groupResource schema.GroupResource
	now           func() time.Time
}

var _ storage.Interface = &store{}

// New returns a store for one resource. Keys passed to the store must start
// with resourcePrefix. prefix separates the data of different API servers
// in one table, as the etcd path prefix does.
func New(cfg Config, codec runtime.Codec, newFunc, newListFunc func() runtime.Object,
	prefix, resourcePrefix string, groupResource schema.GroupResource) (storage.Interface, error) {
	return newStore(cfg, codec, newFunc, newListFunc, prefix, resourcePrefix, groupResource)
}

func newStore(cfg Config, codec runtime.Codec, newFunc, newListFunc func() runtime.Object,
	prefix, resourcePrefix string, groupResource schema.GroupResource) (*store, error) {
	if cfg.Client == nil {
		return nil, errors.New("dynamo: Config.Client is nil")
	}
	if resourcePrefix == "" || resourcePrefix == "/" || resourcePrefix[0] != '/' {
		return nil, fmt.Errorf("dynamo: invalid resource prefix %q", resourcePrefix)
	}
	s := &store{
		client:         cfg.Client,
		table:          cfg.Table,
		retention:      cfg.EventRetention,
		pollInterval:   cfg.PollInterval,
		codec:          codec,
		versioner:      storage.APIObjectVersioner{},
		newFunc:        newFunc,
		newListFunc:    newListFunc,
		resourcePrefix: resourcePrefix,
		resource:       joinPrefix(prefix, resourcePrefix),
		groupResource:  groupResource,
		now:            time.Now,
	}
	if s.table == "" {
		s.table = DefaultTable
	}
	if s.retention == 0 {
		s.retention = DefaultEventRetention
	}
	if s.pollInterval == 0 {
		s.pollInterval = DefaultPollInterval
	}
	return s, nil
}

func (s *store) Versioner() storage.Versioner { return s.versioner }

func (s *store) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64) error {
	return errors.ErrUnsupported
}

func (s *store) Delete(ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions,
	validateDeletion storage.ValidateObjectFunc, cachedExistingObject runtime.Object, opts storage.DeleteOptions) error {
	return errors.ErrUnsupported
}

func (s *store) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	return nil, errors.ErrUnsupported
}

func (s *store) Get(ctx context.Context, key string, opts storage.GetOptions, out runtime.Object) error {
	return errors.ErrUnsupported
}

func (s *store) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	return errors.ErrUnsupported
}

func (s *store) GuaranteedUpdate(ctx context.Context, key string, destination runtime.Object, ignoreNotFound bool,
	preconditions *storage.Preconditions, tryUpdate storage.UpdateFunc, cachedExistingObject runtime.Object) error {
	return errors.ErrUnsupported
}

func (s *store) Stats(ctx context.Context) (storage.Stats, error) {
	return storage.Stats{}, errors.ErrUnsupported
}

func (s *store) ReadinessCheck() error { return errors.ErrUnsupported }

func (s *store) RequestWatchProgress(ctx context.Context) error { return errors.ErrUnsupported }

func (s *store) GetCurrentResourceVersion(ctx context.Context) (uint64, error) {
	return 0, errors.ErrUnsupported
}

func (s *store) EnableResourceSizeEstimation(storage.KeysFunc) error { return errors.ErrUnsupported }

func (s *store) CompactRevision() int64 { return 0 }

// joinPrefix joins the path prefix and the resource prefix, so that
// "/registry" and "/devices" give "/registry/devices".
func joinPrefix(prefix, resourcePrefix string) string {
	if prefix == "" || prefix == "/" {
		return resourcePrefix
	}
	if prefix[0] != '/' {
		prefix = "/" + prefix
	}
	for len(prefix) > 1 && prefix[len(prefix)-1] == '/' {
		prefix = prefix[:len(prefix)-1]
	}
	return prefix + resourcePrefix
}
