package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/grafana/grafana/pkg/internal/infra/log"
	"github.com/grafana/grafana/pkg/internal/models"
	"github.com/grafana/grafana/pkg/internal/plugins/backendplugin"
	"github.com/grafana/grafana/pkg/internal/plugins/backendplugin/grpcplugin"
	"github.com/grafana/grafana/pkg/internal/util/errutil"
)

// DataSourcePlugin contains all metadata about a datasource plugin
type DataSourcePlugin struct {
	FrontendPluginBase
	Annotations  bool              `json:"annotations"`
	Metrics      bool              `json:"metrics"`
	Alerting     bool              `json:"alerting"`
	Explore      bool              `json:"explore"`
	Table        bool              `json:"tables"`
	Logs         bool              `json:"logs"`
	Tracing      bool              `json:"tracing"`
	QueryOptions map[string]bool   `json:"queryOptions,omitempty"`
	BuiltIn      bool              `json:"builtIn,omitempty"`
	Mixed        bool              `json:"mixed,omitempty"`
	Routes       []*AppPluginRoute `json:"routes"`
	Streaming    bool              `json:"streaming"`

	Backend    bool   `json:"backend,omitempty"`
	Executable string `json:"executable,omitempty"`
	SDK        bool   `json:"sdk,omitempty"`

	client       *grpcplugin.Client
	legacyClient *grpcplugin.LegacyClient
	logger       log.Logger
}

func (p *DataSourcePlugin) Load(decoder *json.Decoder, base *PluginBase, backendPluginManager backendplugin.Manager) (
	interface{}, error) {
	if err := decoder.Decode(p); err != nil {
		return nil, errutil.Wrapf(err, "Failed to decode datasource plugin")
	}

	if p.Backend {
		cmd := ComposePluginStartCommand(p.Executable)
		fullpath := filepath.Join(base.PluginDir, cmd)
		factory := grpcplugin.NewBackendPlugin(p.Id, fullpath, grpcplugin.PluginStartFuncs{
			OnLegacyStart: p.onLegacyPluginStart,
			OnStart:       p.onPluginStart,
		})
		if err := backendPluginManager.RegisterAndStart(context.Background(), p.Id, factory); err != nil {
			return nil, errutil.Wrapf(err, "failed to register backend plugin")
		}
	}

	return p, nil
}

func (p *DataSourcePlugin) DataQuery(ctx context.Context, dsInfo *models.DataSource, query DataQuery) (DataResponse, error) {
	if !p.CanHandleDataQueries() {
		return DataResponse{}, fmt.Errorf("plugin %q can't handle data queries", p.Id)
	}

	if p.client != nil {
		endpoint := newDataSourcePluginWrapperV2(p.logger, p.Id, p.Type, p.client.DataPlugin)
		return endpoint.Query(ctx, dsInfo, query)
	}

	endpoint := newDataSourcePluginWrapper(p.logger, p.legacyClient.DatasourcePlugin)
	return endpoint.Query(ctx, dsInfo, query)
}

func (p *DataSourcePlugin) onLegacyPluginStart(pluginID string, client *grpcplugin.LegacyClient, logger log.Logger) error {
	p.legacyClient = client
	p.logger = logger

	return nil
}

func (p *DataSourcePlugin) CanHandleDataQueries() bool {
	return p.client != nil || p.legacyClient != nil
}

func (p *DataSourcePlugin) onPluginStart(pluginID string, client *grpcplugin.Client, logger log.Logger) error {
	if client.DataPlugin == nil {
		return nil
	}

	p.client = client
	p.logger = logger

	return nil
}
