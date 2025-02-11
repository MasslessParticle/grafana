package plugins

import (
	"context"
	"encoding/json"
	"path/filepath"

	pluginModel "github.com/grafana/grafana-plugin-model/go/renderer"
	"github.com/grafana/grafana/pkg/internal/infra/log"
	"github.com/grafana/grafana/pkg/internal/plugins/backendplugin"
	"github.com/grafana/grafana/pkg/internal/plugins/backendplugin/grpcplugin"
	"github.com/grafana/grafana/pkg/internal/plugins/backendplugin/pluginextensionv2"
	"github.com/grafana/grafana/pkg/internal/util/errutil"
)

type RendererPlugin struct {
	FrontendPluginBase

	Executable           string `json:"executable,omitempty"`
	GrpcPluginV1         pluginModel.RendererPlugin
	GrpcPluginV2         pluginextensionv2.RendererPlugin
	backendPluginManager backendplugin.Manager
}

func (r *RendererPlugin) Load(decoder *json.Decoder, base *PluginBase,
	backendPluginManager backendplugin.Manager) (interface{}, error) {
	if err := decoder.Decode(r); err != nil {
		return nil, err
	}

	r.backendPluginManager = backendPluginManager

	cmd := ComposePluginStartCommand("plugin_start")
	fullpath := filepath.Join(base.PluginDir, cmd)
	factory := grpcplugin.NewRendererPlugin(r.Id, fullpath, grpcplugin.PluginStartFuncs{
		OnLegacyStart: r.onLegacyPluginStart,
		OnStart:       r.onPluginStart,
	})
	if err := backendPluginManager.Register(r.Id, factory); err != nil {
		return nil, errutil.Wrapf(err, "failed to register backend plugin")
	}

	return r, nil
}

func (r *RendererPlugin) Start(ctx context.Context) error {
	if err := r.backendPluginManager.StartPlugin(ctx, r.Id); err != nil {
		return errutil.Wrapf(err, "Failed to start renderer plugin")
	}

	return nil
}

func (r *RendererPlugin) onLegacyPluginStart(pluginID string, client *grpcplugin.LegacyClient, logger log.Logger) error {
	r.GrpcPluginV1 = client.RendererPlugin
	return nil
}

func (r *RendererPlugin) onPluginStart(pluginID string, client *grpcplugin.Client, logger log.Logger) error {
	r.GrpcPluginV2 = client.RendererPlugin
	return nil
}
