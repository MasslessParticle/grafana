package datasourceproxy

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/grafana/grafana/pkg/internal/api/datasource"
	"github.com/grafana/grafana/pkg/internal/api/pluginproxy"
	"github.com/grafana/grafana/pkg/internal/infra/metrics"
	"github.com/grafana/grafana/pkg/internal/models"
	"github.com/grafana/grafana/pkg/internal/plugins"
	"github.com/grafana/grafana/pkg/internal/registry"
	"github.com/grafana/grafana/pkg/internal/services/datasources"
	"github.com/grafana/grafana/pkg/internal/setting"
)

func init() {
	registry.RegisterService(&DatasourceProxyService{})
}

type DatasourceProxyService struct {
	DatasourceCache        datasources.CacheService      `inject:""`
	PluginRequestValidator models.PluginRequestValidator `inject:""`
	PluginManager          plugins.Manager               `inject:""`
	Cfg                    *setting.Cfg                  `inject:""`
}

func (p *DatasourceProxyService) Init() error {
	return nil
}

func (p *DatasourceProxyService) ProxyDataSourceRequest(c *models.ReqContext) {
	p.ProxyDatasourceRequestWithID(c, c.ParamsInt64(":id"))
}

func (p *DatasourceProxyService) ProxyDatasourceRequestWithID(c *models.ReqContext, dsID int64) {
	c.TimeRequest(metrics.MDataSourceProxyReqTimer)

	ds, err := p.DatasourceCache.GetDatasource(dsID, c.SignedInUser, c.SkipCache)
	if err != nil {
		if errors.Is(err, models.ErrDataSourceAccessDenied) {
			c.JsonApiErr(http.StatusForbidden, "Access denied to datasource", err)
			return
		}
		c.JsonApiErr(http.StatusInternalServerError, "Unable to load datasource meta data", err)
		return
	}

	err = p.PluginRequestValidator.Validate(ds.Url, c.Req.Request)
	if err != nil {
		c.JsonApiErr(http.StatusForbidden, "Access denied", err)
		return
	}

	// find plugin
	plugin := p.PluginManager.GetDataSource(ds.Type)
	if plugin == nil {
		c.JsonApiErr(http.StatusInternalServerError, "Unable to find datasource plugin", err)
		return
	}

	proxyPath := getProxyPath(c)
	proxy, err := pluginproxy.NewDataSourceProxy(ds, plugin, c, proxyPath, p.Cfg)
	if err != nil {
		if errors.Is(err, datasource.URLValidationError{}) {
			c.JsonApiErr(http.StatusBadRequest, fmt.Sprintf("Invalid data source URL: %q", ds.Url), err)
		} else {
			c.JsonApiErr(http.StatusInternalServerError, "Failed creating data source proxy", err)
		}
		return
	}
	proxy.HandleRequest()
}

var proxyPathRegexp = regexp.MustCompile(`^\/api\/datasources\/proxy\/[\d]+\/?`)

func extractProxyPath(originalRawPath string) string {
	return proxyPathRegexp.ReplaceAllString(originalRawPath, "")
}

func getProxyPath(c *models.ReqContext) string {
	return extractProxyPath(c.Req.URL.EscapedPath())
}
