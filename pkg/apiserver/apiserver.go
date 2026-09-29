package apiserver

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"

	"github.com/phoban01/solas/pkg/apis/solas"
	"github.com/phoban01/solas/pkg/registry/solas/device"
	"github.com/phoban01/solas/pkg/registry/solas/member"
)

// Config is the configuration of the solas API server.
type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
}

// CompletedConfig is a Config after Complete.
type CompletedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
}

// Server is the solas API server.
type Server struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

// Complete fills in the defaults of the configuration.
func (c *Config) Complete() CompletedConfig {
	return CompletedConfig{GenericConfig: c.GenericConfig.Complete()}
}

// New returns a server that serves the solas.dev group.
func (c CompletedConfig) New() (*Server, error) {
	generic, err := c.GenericConfig.New("solas-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}
	getter := c.GenericConfig.RESTOptionsGetter
	devices, deviceStatus, err := device.NewREST(Scheme, getter)
	if err != nil {
		return nil, err
	}
	members, memberStatus, err := member.NewREST(Scheme, getter)
	if err != nil {
		return nil, err
	}

	group := genericapiserver.NewDefaultAPIGroupInfo(solas.GroupName, Scheme, metav1.ParameterCodec, Codecs)
	group.VersionedResourcesStorageMap["v1alpha1"] = map[string]rest.Storage{
		"devices":        devices,
		"devices/status": deviceStatus,
		"members":        members,
		"members/status": memberStatus,
	}
	if err := generic.InstallAPIGroup(&group); err != nil {
		return nil, err
	}
	return &Server{GenericAPIServer: generic}, nil
}
