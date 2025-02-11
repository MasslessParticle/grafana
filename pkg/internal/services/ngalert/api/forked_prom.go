package api

import (
	"fmt"

	"github.com/grafana/grafana/pkg/internal/api/response"
	"github.com/grafana/grafana/pkg/internal/models"
	"github.com/grafana/grafana/pkg/internal/services/datasources"
	apimodels "github.com/grafana/grafana/pkg/internal/services/ngalert/api/tooling/definitions"
)

type ForkedPromSvc struct {
	ProxySvc, GrafanaSvc PrometheusApiService
	DatasourceCache      datasources.CacheService
}

// NewForkedProm implements a set of routes that proxy to various Prometheus-compatible backends.
func NewForkedProm(datasourceCache datasources.CacheService, proxy, grafana PrometheusApiService) *ForkedPromSvc {
	return &ForkedPromSvc{
		ProxySvc:        proxy,
		GrafanaSvc:      grafana,
		DatasourceCache: datasourceCache,
	}
}

func (p *ForkedPromSvc) RouteGetAlertStatuses(ctx *models.ReqContext) response.Response {
	t, err := backendType(ctx, p.DatasourceCache)
	if err != nil {
		return response.Error(400, err.Error(), nil)
	}

	switch t {
	case apimodels.GrafanaBackend:
		return p.GrafanaSvc.RouteGetAlertStatuses(ctx)
	case apimodels.LoTexRulerBackend:
		return p.ProxySvc.RouteGetAlertStatuses(ctx)
	default:
		return response.Error(400, fmt.Sprintf("unexpected backend type (%v)", t), nil)
	}
}

func (p *ForkedPromSvc) RouteGetRuleStatuses(ctx *models.ReqContext) response.Response {
	t, err := backendType(ctx, p.DatasourceCache)
	if err != nil {
		return response.Error(400, err.Error(), nil)
	}

	switch t {
	case apimodels.GrafanaBackend:
		return p.GrafanaSvc.RouteGetRuleStatuses(ctx)
	case apimodels.LoTexRulerBackend:
		return p.ProxySvc.RouteGetRuleStatuses(ctx)
	default:
		return response.Error(400, fmt.Sprintf("unexpected backend type (%v)", t), nil)
	}
}
