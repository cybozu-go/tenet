package client

import (
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCachingClient is an alternative implementation of controller-runtime's
// default client for manager.Manager.
// https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/cluster#DefaultNewClient
//
// The only difference is that this implementation sets `CacheUnstructured` to `true` to
// cache unstructured objects.
func NewCachingClient(cache cache.Cache, config *rest.Config, options client.Options, uncachedObjects ...client.Object) (client.Client, error) {
	c, err := client.New(config, options)
	if err != nil {
		return nil, err
	}

	return client.NewDelegatingClient(client.NewDelegatingClientInput{
		CacheReader:       cache,
		Client:            c,
		UncachedObjects:   uncachedObjects,
		CacheUnstructured: true,
	})
}
