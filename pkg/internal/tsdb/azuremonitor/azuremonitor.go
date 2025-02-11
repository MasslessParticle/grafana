package azuremonitor

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/grafana/grafana/pkg/internal/infra/log"
	"github.com/grafana/grafana/pkg/internal/models"
	"github.com/grafana/grafana/pkg/internal/plugins"
	"github.com/grafana/grafana/pkg/internal/registry"
	"github.com/grafana/grafana/pkg/internal/setting"
)

var (
	azlog           = log.New("tsdb.azuremonitor")
	legendKeyFormat = regexp.MustCompile(`\{\{\s*(.+?)\s*\}\}`)
)

func init() {
	registry.Register(&registry.Descriptor{
		Name:         "AzureMonitorService",
		InitPriority: registry.Low,
		Instance:     &Service{},
	})
}

type Service struct {
	PluginManager plugins.Manager `inject:""`
	Cfg           *setting.Cfg    `inject:""`
}

func (s *Service) Init() error {
	return nil
}

// AzureMonitorExecutor executes queries for the Azure Monitor datasource - all four services
type AzureMonitorExecutor struct {
	httpClient    *http.Client
	dsInfo        *models.DataSource
	pluginManager plugins.Manager
	cfg           *setting.Cfg
}

// NewAzureMonitorExecutor initializes a http client
//nolint: staticcheck // plugins.DataPlugin deprecated
func (s *Service) NewExecutor(dsInfo *models.DataSource) (plugins.DataPlugin, error) {
	httpClient, err := dsInfo.GetHttpClient()
	if err != nil {
		return nil, err
	}

	return &AzureMonitorExecutor{
		httpClient:    httpClient,
		dsInfo:        dsInfo,
		pluginManager: s.PluginManager,
		cfg:           s.Cfg,
	}, nil
}

// Query takes in the frontend queries, parses them into the query format
// expected by chosen Azure Monitor service (Azure Monitor, App Insights etc.)
// executes the queries against the API and parses the response into
// the right format
//nolint: staticcheck // plugins.DataPlugin deprecated
func (e *AzureMonitorExecutor) DataQuery(ctx context.Context, dsInfo *models.DataSource,
	tsdbQuery plugins.DataQuery) (plugins.DataResponse, error) {
	var err error

	var azureMonitorQueries []plugins.DataSubQuery
	var applicationInsightsQueries []plugins.DataSubQuery
	var azureLogAnalyticsQueries []plugins.DataSubQuery
	var insightsAnalyticsQueries []plugins.DataSubQuery

	for _, query := range tsdbQuery.Queries {
		queryType := query.Model.Get("queryType").MustString("")

		switch queryType {
		case "Azure Monitor":
			azureMonitorQueries = append(azureMonitorQueries, query)
		case "Application Insights":
			applicationInsightsQueries = append(applicationInsightsQueries, query)
		case "Azure Log Analytics":
			azureLogAnalyticsQueries = append(azureLogAnalyticsQueries, query)
		case "Insights Analytics":
			insightsAnalyticsQueries = append(insightsAnalyticsQueries, query)
		default:
			return plugins.DataResponse{}, fmt.Errorf("alerting not supported for %q", queryType)
		}
	}

	azDatasource := &AzureMonitorDatasource{
		httpClient:    e.httpClient,
		dsInfo:        e.dsInfo,
		pluginManager: e.pluginManager,
		cfg:           e.cfg,
	}

	aiDatasource := &ApplicationInsightsDatasource{
		httpClient:    e.httpClient,
		dsInfo:        e.dsInfo,
		pluginManager: e.pluginManager,
		cfg:           e.cfg,
	}

	alaDatasource := &AzureLogAnalyticsDatasource{
		httpClient:    e.httpClient,
		dsInfo:        e.dsInfo,
		pluginManager: e.pluginManager,
		cfg:           e.cfg,
	}

	iaDatasource := &InsightsAnalyticsDatasource{
		httpClient:    e.httpClient,
		dsInfo:        e.dsInfo,
		pluginManager: e.pluginManager,
		cfg:           e.cfg,
	}

	azResult, err := azDatasource.executeTimeSeriesQuery(ctx, azureMonitorQueries, *tsdbQuery.TimeRange)
	if err != nil {
		return plugins.DataResponse{}, err
	}

	aiResult, err := aiDatasource.executeTimeSeriesQuery(ctx, applicationInsightsQueries, *tsdbQuery.TimeRange)
	if err != nil {
		return plugins.DataResponse{}, err
	}

	alaResult, err := alaDatasource.executeTimeSeriesQuery(ctx, azureLogAnalyticsQueries, *tsdbQuery.TimeRange)
	if err != nil {
		return plugins.DataResponse{}, err
	}

	iaResult, err := iaDatasource.executeTimeSeriesQuery(ctx, insightsAnalyticsQueries, *tsdbQuery.TimeRange)
	if err != nil {
		return plugins.DataResponse{}, err
	}

	for k, v := range aiResult.Results {
		azResult.Results[k] = v
	}

	for k, v := range alaResult.Results {
		azResult.Results[k] = v
	}

	for k, v := range iaResult.Results {
		azResult.Results[k] = v
	}

	return azResult, nil
}
