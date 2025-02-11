package grpcplugin

import (
	"context"
	"errors"
	"sync"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana/pkg/internal/infra/log"
	"github.com/grafana/grafana/pkg/internal/plugins/backendplugin"
	"github.com/hashicorp/go-plugin"
)

type pluginClient interface {
	backend.CollectMetricsHandler
	backend.CheckHealthHandler
	backend.CallResourceHandler
	backend.StreamHandler
}

type grpcPlugin struct {
	descriptor     PluginDescriptor
	clientFactory  func() *plugin.Client
	client         *plugin.Client
	pluginClient   pluginClient
	logger         log.Logger
	mutex          sync.RWMutex
	decommissioned bool
}

// newPlugin allocates and returns a new gRPC (external) backendplugin.Plugin.
func newPlugin(descriptor PluginDescriptor) backendplugin.PluginFactoryFunc {
	return func(pluginID string, logger log.Logger, env []string) (backendplugin.Plugin, error) {
		return &grpcPlugin{
			descriptor: descriptor,
			logger:     logger,
			clientFactory: func() *plugin.Client {
				return plugin.NewClient(newClientConfig(descriptor.executablePath, env, logger, descriptor.versionedPlugins))
			},
		}, nil
	}
}

func (p *grpcPlugin) PluginID() string {
	return p.descriptor.pluginID
}

func (p *grpcPlugin) Logger() log.Logger {
	return p.logger
}

func (p *grpcPlugin) Start(ctx context.Context) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.client = p.clientFactory()
	rpcClient, err := p.client.Client()
	if err != nil {
		return err
	}

	if p.client.NegotiatedVersion() > 1 {
		p.pluginClient, err = newClientV2(p.descriptor, p.logger, rpcClient)
		if err != nil {
			return err
		}
	} else {
		p.pluginClient, err = newClientV1(p.descriptor, p.logger, rpcClient)
		if err != nil {
			return err
		}
	}

	if p.pluginClient == nil {
		return errors.New("no compatible plugin implementation found")
	}

	return nil
}

func (p *grpcPlugin) Stop(ctx context.Context) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.client != nil {
		p.client.Kill()
	}
	return nil
}

func (p *grpcPlugin) IsManaged() bool {
	return p.descriptor.managed
}

func (p *grpcPlugin) Exited() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if p.client != nil {
		return p.client.Exited()
	}
	return true
}

func (p *grpcPlugin) Decommission() error {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	p.decommissioned = true

	return nil
}

func (p *grpcPlugin) IsDecommissioned() bool {
	return p.decommissioned
}

func (p *grpcPlugin) getPluginClient() (pluginClient, bool) {
	p.mutex.RLock()
	if p.client == nil || p.client.Exited() || p.pluginClient == nil {
		p.mutex.RUnlock()
		return nil, false
	}
	pluginClient := p.pluginClient
	p.mutex.RUnlock()
	return pluginClient, true
}

func (p *grpcPlugin) CollectMetrics(ctx context.Context) (*backend.CollectMetricsResult, error) {
	pluginClient, ok := p.getPluginClient()
	if !ok {
		return nil, backendplugin.ErrPluginUnavailable
	}
	return pluginClient.CollectMetrics(ctx)
}

func (p *grpcPlugin) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	pluginClient, ok := p.getPluginClient()
	if !ok {
		return nil, backendplugin.ErrPluginUnavailable
	}
	return pluginClient.CheckHealth(ctx, req)
}

func (p *grpcPlugin) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	pluginClient, ok := p.getPluginClient()
	if !ok {
		return backendplugin.ErrPluginUnavailable
	}
	return pluginClient.CallResource(ctx, req, sender)
}

func (p *grpcPlugin) SubscribeStream(ctx context.Context, request *backend.SubscribeStreamRequest) (*backend.SubscribeStreamResponse, error) {
	pluginClient, ok := p.getPluginClient()
	if !ok {
		return nil, backendplugin.ErrPluginUnavailable
	}
	return pluginClient.SubscribeStream(ctx, request)
}

func (p *grpcPlugin) PublishStream(ctx context.Context, request *backend.PublishStreamRequest) (*backend.PublishStreamResponse, error) {
	pluginClient, ok := p.getPluginClient()
	if !ok {
		return nil, backendplugin.ErrPluginUnavailable
	}
	return pluginClient.PublishStream(ctx, request)
}

func (p *grpcPlugin) RunStream(ctx context.Context, req *backend.RunStreamRequest, sender backend.StreamPacketSender) error {
	pluginClient, ok := p.getPluginClient()
	if !ok {
		return backendplugin.ErrPluginUnavailable
	}
	return pluginClient.RunStream(ctx, req, sender)
}
