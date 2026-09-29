package apiserver

import (
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/storage"
	cacherstorage "k8s.io/apiserver/pkg/storage/cacher"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/apiserver/pkg/storage/storagebackend/factory"
	"k8s.io/client-go/tools/cache"

	"github.com/phoban01/solas/pkg/storage/dynamo"
)

// eventsHistoryWindow is the history of the watch cache. The cacher needs
// at least 75 seconds.
const eventsHistoryWindow = 5 * time.Minute

// RESTOptionsGetter gives each resource a DynamoDB store behind the watch
// cache.
type RESTOptionsGetter struct {
	Dynamo dynamo.Config
	Codec  runtime.Codec
	// EncodeVersioner picks the version that objects are stored in.
	EncodeVersioner runtime.GroupVersioner
	// Prefix separates the data of different API servers in one table.
	Prefix string
}

var _ generic.RESTOptionsGetter = RESTOptionsGetter{}

// GetRESTOptions returns the options for one resource.
func (g RESTOptionsGetter) GetRESTOptions(resource schema.GroupResource, _ runtime.Object) (generic.RESTOptions, error) {
	return generic.RESTOptions{
		StorageConfig: &storagebackend.ConfigForResource{
			Config: storagebackend.Config{
				Prefix:          g.Prefix,
				Codec:           g.Codec,
				EncodeVersioner: g.EncodeVersioner,
				// The watch cache keeps this much history. It must stay below
				// the event retention of the store.
				EventsHistoryWindow: eventsHistoryWindow,
			},
			GroupResource: resource,
		},
		Decorator:               g.decorator(),
		EnableGarbageCollection: true,
		DeleteCollectionWorkers: 1,
		ResourcePrefix:          "/" + resource.Group + "/" + resource.Resource,
		CountMetricPollPeriod:   time.Minute,
	}, nil
}

// decorator builds the DynamoDB store and wraps it in the watch cache, as
// generic registry StorageWithCacher does for etcd.
func (g RESTOptionsGetter) decorator() generic.StorageDecorator {
	return func(
		config *storagebackend.ConfigForResource,
		resourcePrefix string,
		keyFunc func(obj runtime.Object) (string, error),
		newFunc func() runtime.Object,
		newListFunc func() runtime.Object,
		getAttrsFunc storage.AttrFunc,
		triggerFuncs storage.IndexerFuncs,
		indexers *cache.Indexers) (storage.Interface, factory.DestroyFunc, error) {
		s, err := dynamo.New(g.Dynamo, config.Codec, newFunc, newListFunc, config.Prefix, resourcePrefix, config.GroupResource)
		if err != nil {
			return nil, func() {}, err
		}
		//= spec/solas.md#4-2-poll
		//# The server SHOULD run one poller for each resource and share it between
		//# watchers.
		cacher, err := cacherstorage.NewCacherFromConfig(cacherstorage.Config{
			Storage:             s,
			EventsHistoryWindow: config.EventsHistoryWindow,
			Versioner:           storage.APIObjectVersioner{},
			GroupResource:       config.GroupResource,
			ResourcePrefix:      resourcePrefix,
			KeyFunc:             keyFunc,
			NewFunc:             newFunc,
			NewListFunc:         newListFunc,
			GetAttrsFunc:        getAttrsFunc,
			IndexerFuncs:        triggerFuncs,
			Indexers:            indexers,
			Codec:               config.Codec,
		})
		if err != nil {
			return nil, func() {}, err
		}
		delegator := cacherstorage.NewCacheDelegator(cacher, s)
		var once sync.Once
		destroy := func() {
			once.Do(func() {
				delegator.Stop()
				cacher.Stop()
			})
		}
		return delegator, destroy, nil
	}
}
