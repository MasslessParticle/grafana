package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"

	apimodels "github.com/grafana/grafana/pkg/internal/services/ngalert/api/tooling/definitions"
	"gopkg.in/yaml.v3"

	"github.com/grafana/grafana/pkg/internal/api/response"
	"github.com/grafana/grafana/pkg/internal/infra/log"
	"github.com/grafana/grafana/pkg/internal/models"
)

var dsTypeToRulerPrefix = map[string]string{
	"prometheus": "/rules",
	"loki":       "/api/prom/rules",
}

type LotexRuler struct {
	log log.Logger
	*AlertingProxy
}

func NewLotexRuler(proxy *AlertingProxy, log log.Logger) *LotexRuler {
	return &LotexRuler{
		log:           log,
		AlertingProxy: proxy,
	}
}

func (r *LotexRuler) RouteDeleteNamespaceRulesConfig(ctx *models.ReqContext) response.Response {
	legacyRulerPrefix, err := r.getPrefix(ctx)
	if err != nil {
		return response.Error(500, err.Error(), nil)
	}
	return r.withReq(
		ctx,
		http.MethodDelete,
		withPath(
			*ctx.Req.URL,
			fmt.Sprintf("%s/%s", legacyRulerPrefix, ctx.Params("Namespace")),
		),
		nil,
		messageExtractor,
		nil,
	)
}

func (r *LotexRuler) RouteDeleteRuleGroupConfig(ctx *models.ReqContext) response.Response {
	legacyRulerPrefix, err := r.getPrefix(ctx)
	if err != nil {
		return response.Error(500, err.Error(), nil)
	}
	return r.withReq(
		ctx,
		http.MethodDelete,
		withPath(
			*ctx.Req.URL,
			fmt.Sprintf(
				"%s/%s/%s",
				legacyRulerPrefix,
				ctx.Params("Namespace"),
				ctx.Params("Groupname"),
			),
		),
		nil,
		messageExtractor,
		nil,
	)
}

func (r *LotexRuler) RouteGetNamespaceRulesConfig(ctx *models.ReqContext) response.Response {
	legacyRulerPrefix, err := r.getPrefix(ctx)
	if err != nil {
		return response.Error(500, err.Error(), nil)
	}
	return r.withReq(
		ctx,
		http.MethodGet,
		withPath(
			*ctx.Req.URL,
			fmt.Sprintf(
				"%s/%s",
				legacyRulerPrefix,
				ctx.Params("Namespace"),
			),
		),
		nil,
		yamlExtractor(apimodels.NamespaceConfigResponse{}),
		nil,
	)
}

func (r *LotexRuler) RouteGetRulegGroupConfig(ctx *models.ReqContext) response.Response {
	legacyRulerPrefix, err := r.getPrefix(ctx)
	if err != nil {
		return response.Error(500, err.Error(), nil)
	}
	return r.withReq(
		ctx,
		http.MethodGet,
		withPath(
			*ctx.Req.URL,
			fmt.Sprintf(
				"%s/%s/%s",
				legacyRulerPrefix,
				ctx.Params("Namespace"),
				ctx.Params("Groupname"),
			),
		),
		nil,
		yamlExtractor(&apimodels.GettableRuleGroupConfig{}),
		nil,
	)
}

func (r *LotexRuler) RouteGetRulesConfig(ctx *models.ReqContext) response.Response {
	legacyRulerPrefix, err := r.getPrefix(ctx)
	if err != nil {
		return response.Error(500, err.Error(), nil)
	}
	return r.withReq(
		ctx,
		http.MethodGet,
		withPath(
			*ctx.Req.URL,
			legacyRulerPrefix,
		),
		nil,
		yamlExtractor(apimodels.NamespaceConfigResponse{}),
		nil,
	)
}

func (r *LotexRuler) RoutePostNameRulesConfig(ctx *models.ReqContext, conf apimodels.PostableRuleGroupConfig) response.Response {
	legacyRulerPrefix, err := r.getPrefix(ctx)
	if err != nil {
		return response.Error(500, err.Error(), nil)
	}
	yml, err := yaml.Marshal(conf)
	if err != nil {
		return response.Error(500, "Failed marshal rule group", err)
	}
	ns := ctx.Params("Namespace")
	u := withPath(*ctx.Req.URL, fmt.Sprintf("%s/%s", legacyRulerPrefix, ns))
	return r.withReq(ctx, http.MethodPost, u, bytes.NewBuffer(yml), jsonExtractor(nil), nil)
}

func (r *LotexRuler) getPrefix(ctx *models.ReqContext) (string, error) {
	ds, err := r.DataProxy.DatasourceCache.GetDatasource(ctx.ParamsInt64("Recipient"), ctx.SignedInUser, ctx.SkipCache)
	if err != nil {
		return "", err
	}
	prefix, ok := dsTypeToRulerPrefix[ds.Type]
	if !ok {
		return "", fmt.Errorf("unexpected datasource type. expecting loki or prometheus")
	}
	return prefix, nil
}

func withPath(u url.URL, newPath string) *url.URL {
	// TODO: handle path escaping
	u.Path = newPath
	return &u
}
