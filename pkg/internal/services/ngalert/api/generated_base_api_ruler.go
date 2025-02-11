/*Package api contains base API implementation of unified alerting
 *
 *Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 *
 *Do not manually edit these files, please find ngalert/api/swagger-codegen/ for commands on how to generate them.
 */

package api

import (
	"net/http"

	"github.com/go-macaron/binding"

	"github.com/grafana/grafana/pkg/internal/api/response"
	"github.com/grafana/grafana/pkg/internal/api/routing"
	"github.com/grafana/grafana/pkg/internal/middleware"
	"github.com/grafana/grafana/pkg/internal/models"
	apimodels "github.com/grafana/grafana/pkg/internal/services/ngalert/api/tooling/definitions"
	"github.com/grafana/grafana/pkg/internal/services/ngalert/metrics"
)

type RulerApiService interface {
	RouteDeleteNamespaceRulesConfig(*models.ReqContext) response.Response
	RouteDeleteRuleGroupConfig(*models.ReqContext) response.Response
	RouteGetNamespaceRulesConfig(*models.ReqContext) response.Response
	RouteGetRulegGroupConfig(*models.ReqContext) response.Response
	RouteGetRulesConfig(*models.ReqContext) response.Response
	RoutePostNameRulesConfig(*models.ReqContext, apimodels.PostableRuleGroupConfig) response.Response
}

func (api *API) RegisterRulerApiEndpoints(srv RulerApiService, m *metrics.Metrics) {
	api.RouteRegister.Group("", func(group routing.RouteRegister) {
		group.Delete(
			toMacaronPath("/api/ruler/{Recipient}/api/v1/rules/{Namespace}"),
			metrics.Instrument(
				http.MethodDelete,
				"/api/ruler/{Recipient}/api/v1/rules/{Namespace}",
				srv.RouteDeleteNamespaceRulesConfig,
				m,
			),
		)
		group.Delete(
			toMacaronPath("/api/ruler/{Recipient}/api/v1/rules/{Namespace}/{Groupname}"),
			metrics.Instrument(
				http.MethodDelete,
				"/api/ruler/{Recipient}/api/v1/rules/{Namespace}/{Groupname}",
				srv.RouteDeleteRuleGroupConfig,
				m,
			),
		)
		group.Get(
			toMacaronPath("/api/ruler/{Recipient}/api/v1/rules/{Namespace}"),
			metrics.Instrument(
				http.MethodGet,
				"/api/ruler/{Recipient}/api/v1/rules/{Namespace}",
				srv.RouteGetNamespaceRulesConfig,
				m,
			),
		)
		group.Get(
			toMacaronPath("/api/ruler/{Recipient}/api/v1/rules/{Namespace}/{Groupname}"),
			metrics.Instrument(
				http.MethodGet,
				"/api/ruler/{Recipient}/api/v1/rules/{Namespace}/{Groupname}",
				srv.RouteGetRulegGroupConfig,
				m,
			),
		)
		group.Get(
			toMacaronPath("/api/ruler/{Recipient}/api/v1/rules"),
			metrics.Instrument(
				http.MethodGet,
				"/api/ruler/{Recipient}/api/v1/rules",
				srv.RouteGetRulesConfig,
				m,
			),
		)
		group.Post(
			toMacaronPath("/api/ruler/{Recipient}/api/v1/rules/{Namespace}"),
			binding.Bind(apimodels.PostableRuleGroupConfig{}),
			metrics.Instrument(
				http.MethodPost,
				"/api/ruler/{Recipient}/api/v1/rules/{Namespace}",
				srv.RoutePostNameRulesConfig,
				m,
			),
		)
	}, middleware.ReqSignedIn)
}
