package contexthandler

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/grafana/grafana/pkg/internal/bus"
	"github.com/grafana/grafana/pkg/internal/infra/log"
	"github.com/grafana/grafana/pkg/internal/infra/remotecache"
	"github.com/grafana/grafana/pkg/internal/models"
	"github.com/grafana/grafana/pkg/internal/registry"
	"github.com/grafana/grafana/pkg/internal/services/auth"
	"github.com/grafana/grafana/pkg/internal/services/auth/jwt"
	"github.com/grafana/grafana/pkg/internal/services/contexthandler/authproxy"
	"github.com/grafana/grafana/pkg/internal/services/rendering"
	"github.com/grafana/grafana/pkg/internal/services/sqlstore"
	"github.com/grafana/grafana/pkg/internal/setting"
	"github.com/stretchr/testify/require"
	macaron "gopkg.in/macaron.v1"
)

// Test initContextWithAuthProxy with a cached user ID that is no longer valid.
//
// In this case, the cache entry should be ignored/cleared and another attempt should be done to sign the user
// in without cache.
func TestInitContextWithAuthProxy_CachedInvalidUserID(t *testing.T) {
	const name = "markelog"
	const userID = int64(1)
	const orgID = int64(4)

	upsertHandler := func(cmd *models.UpsertUserCommand) error {
		require.Equal(t, name, cmd.ExternalUser.Login)
		cmd.Result = &models.User{Id: userID}
		return nil
	}
	getUserHandler := func(ctx context.Context, query *models.GetSignedInUserQuery) error {
		// Simulate that the cached user ID is stale
		if query.UserId != userID {
			return models.ErrUserNotFound
		}

		query.Result = &models.SignedInUser{
			UserId: userID,
			OrgId:  orgID,
		}
		return nil
	}
	bus.AddHandler("", upsertHandler)
	bus.AddHandlerCtx("", getUserHandler)
	t.Cleanup(func() {
		bus.ClearBusHandlers()
	})

	svc := getContextHandler(t)

	req, err := http.NewRequest("POST", "http://example.com", nil)
	require.NoError(t, err)
	ctx := &models.ReqContext{
		Context: &macaron.Context{
			Req: macaron.Request{
				Request: req,
			},
			Data: map[string]interface{}{},
		},
		Logger: log.New("Test"),
	}
	req.Header.Set(svc.Cfg.AuthProxyHeaderName, name)
	h, err := authproxy.HashCacheKey(name)
	require.NoError(t, err)
	key := fmt.Sprintf(authproxy.CachePrefix, h)

	t.Logf("Injecting stale user ID in cache with key %q", key)
	err = svc.RemoteCache.Set(key, int64(33), 0)
	require.NoError(t, err)

	authEnabled := svc.initContextWithAuthProxy(ctx, orgID)
	require.True(t, authEnabled)

	require.Equal(t, userID, ctx.SignedInUser.UserId)
	require.True(t, ctx.IsSignedIn)

	i, err := svc.RemoteCache.Get(key)
	require.NoError(t, err)
	require.Equal(t, userID, i.(int64))
}

type fakeRenderService struct {
	rendering.Service
}

func (s *fakeRenderService) Init() error {
	return nil
}

func getContextHandler(t *testing.T) *ContextHandler {
	t.Helper()

	sqlStore := sqlstore.InitTestDB(t)
	remoteCacheSvc := &remotecache.RemoteCache{}

	cfg := setting.NewCfg()
	cfg.RemoteCacheOptions = &setting.RemoteCacheOptions{
		Name: "database",
	}
	cfg.AuthProxyHeaderName = "X-Killa"
	cfg.AuthProxyEnabled = true
	cfg.AuthProxyHeaderProperty = "username"
	userAuthTokenSvc := auth.NewFakeUserAuthTokenService()
	renderSvc := &fakeRenderService{}
	authJWTSvc := models.NewFakeJWTService()
	svc := &ContextHandler{}

	err := registry.BuildServiceGraph([]interface{}{cfg}, []*registry.Descriptor{
		{
			Name:     sqlstore.ServiceName,
			Instance: sqlStore,
		},
		{
			Name:     remotecache.ServiceName,
			Instance: remoteCacheSvc,
		},
		{
			Name:     auth.ServiceName,
			Instance: userAuthTokenSvc,
		},
		{
			Name:     rendering.ServiceName,
			Instance: renderSvc,
		},
		{
			Name:     jwt.ServiceName,
			Instance: authJWTSvc,
		},
		{
			Name:     ServiceName,
			Instance: svc,
		},
	})
	require.NoError(t, err)

	return svc
}
