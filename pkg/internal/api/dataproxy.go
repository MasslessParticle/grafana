package api

import "github.com/grafana/grafana/pkg/internal/models"

func (hs *HTTPServer) ProxyDataSourceRequest(c *models.ReqContext) {
	hs.DataProxy.ProxyDataSourceRequest(c)
}
