package restclient

import (
	"fmt"
	
	"context"
	
	"github.com/coffeehc/microserviceboot/base"
	"github.com/coffeehc/commons/httpcommons/client"
	"github.com/coffeehc/microserviceboot/loadbalancer"
	"github.com/coffeehc/microserviceboot/consultool"
	"github.com/hashicorp/consul/api"
	"github.com/coffeehc/microserviceboot/base/restbase"
)

type ServiceClient interface {
	GetServiceName() string
	GetBaseUrl() string
	GetHttpClient() client.HTTPClient
	BuildRequest(endpintMeta restbase.EndpointMeta,query string) (client.HTTPRequest,error)
}

func NewServiceClient(serviceInfo base.ServiceInfo, httpClientConfig *client.HTTPClientOptions, discoveryConfig interface{}) (ServiceClient, base.Error) {
	if serviceInfo == nil {
		return nil, base.NewError(base.ErrCodeBaseSystemNil, "rest client", "serviceInfo is nil")
	}
	if discoveryConfig == nil {
		return nil, base.NewError(base.ErrCodeBaseSystemNil, "rest client", "discoveryConfig is nil")
	}
	if httpClientConfig == nil {
		httpClientConfig = &client.HTTPClientOptions{}
	}
	rootCxt := context.Background()
	var balancer loadbalancer.Balancer
	var baseURL string
	var err base.Error
	switch c := discoveryConfig.(type) {
	case string: //host
		if c == "" {
			return nil, base.NewError(base.ErrCodeBaseSystemNil, "rest client", "discoveryConfig is a addrs")
		}
		balancer, err = loadbalancer.NewAddrArrayBalancer([]string{c})
		if err != nil {
			return nil, base.NewErrorWrapper("rest client",0, err)
		}
		baseURL = fmt.Sprintf("%s://%s", serviceInfo.GetScheme(), c)
	case *api.Client:
		balancer, err = consultool.NewConsulBalancer(rootCxt, c, serviceInfo)
		if err != nil {
			return nil, err
		}
		baseURL = fmt.Sprintf("%s://%s.%s.service", serviceInfo.GetScheme(), serviceInfo.GetServiceTag(), serviceInfo.GetServiceName())
	}
	restClient := newHttpClient(rootCxt, serviceInfo, balancer, httpClientConfig)
	return &_ServiceClient{
		_restClient:restClient,
		client:   client.NewHTTPClient(restClient.options,restClient.transport)   ,
		serviceInfo: serviceInfo,
		baseURL:baseURL,
	}, nil
}

type _ServiceClient struct {
	_restClient *restClient
	client      client.HTTPClient
	serviceInfo base.ServiceInfo
	baseURL     string
}

func (sc *_ServiceClient) GetServiceName() string {
	return sc.serviceInfo.GetServiceName()
}

func (sc *_ServiceClient) GetBaseUrl() string {
	return sc.baseURL
}

func (sc *_ServiceClient) GetHttpClient() client.HTTPClient {
	return sc.client
}

func (sc *_ServiceClient)BuildRequest(endpintMeta restbase.EndpointMeta,query string) (client.HTTPRequest,error) {
	return  client.NewHTTPRequest(string(endpintMeta.Method),fmt.Sprintf("%s/%s?%s", sc.baseURL, endpintMeta.Path,query))
}


