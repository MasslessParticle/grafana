package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/grafana/grafana/pkg/internal/bus"
	"github.com/grafana/grafana/pkg/internal/models"
	"github.com/grafana/grafana/pkg/internal/setting"
	"gopkg.in/macaron.v1"
)

// OrgRedirect changes org and redirects users if the
// querystring `orgId` doesn't match the active org.
func OrgRedirect(cfg *setting.Cfg) macaron.Handler {
	return func(res http.ResponseWriter, req *http.Request, c *macaron.Context) {
		orgIdValue := req.URL.Query().Get("orgId")
		orgId, err := strconv.ParseInt(orgIdValue, 10, 64)

		if err != nil || orgId == 0 {
			return
		}

		ctx, ok := c.Data["ctx"].(*models.ReqContext)
		if !ok || !ctx.IsSignedIn {
			return
		}

		if orgId == ctx.OrgId {
			return
		}

		cmd := models.SetUsingOrgCommand{UserId: ctx.UserId, OrgId: orgId}
		if err := bus.Dispatch(&cmd); err != nil {
			if ctx.IsApiRequest() {
				ctx.JsonApiErr(404, "Not found", nil)
			} else {
				ctx.Error(404, "Not found")
			}

			return
		}

		newURL := fmt.Sprintf("%s%s?%s", cfg.AppURL, strings.TrimPrefix(c.Req.URL.Path, "/"), c.Req.URL.Query().Encode())
		c.Redirect(newURL, 302)
	}
}
