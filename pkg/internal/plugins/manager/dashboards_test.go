package manager

import (
	"testing"

	"github.com/grafana/grafana/pkg/internal/bus"
	"github.com/grafana/grafana/pkg/internal/components/simplejson"
	"github.com/grafana/grafana/pkg/internal/models"
	"github.com/grafana/grafana/pkg/internal/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPluginDashboards(t *testing.T) {
	pm := newManager(&setting.Cfg{
		FeatureToggles: map[string]bool{},
		PluginSettings: setting.PluginSettings{
			"test-app": map[string]string{
				"path": "testdata/test-app",
			},
		},
	})
	err := pm.Init()
	require.NoError(t, err)

	bus.AddHandler("test", func(query *models.GetDashboardQuery) error {
		if query.Slug == "nginx-connections" {
			dash := models.NewDashboard("Nginx Connections")
			dash.Data.Set("revision", "1.1")
			query.Result = dash
			return nil
		}

		return models.ErrDashboardNotFound
	})

	bus.AddHandler("test", func(query *models.GetDashboardsByPluginIdQuery) error {
		var data = simplejson.New()
		data.Set("title", "Nginx Connections")
		data.Set("revision", 22)

		query.Result = []*models.Dashboard{
			{Slug: "nginx-connections", Data: data},
		}
		return nil
	})

	dashboards, err := pm.GetPluginDashboards(1, "test-app")
	require.NoError(t, err)

	assert.Len(t, dashboards, 2)
	assert.Equal(t, "Nginx Connections", dashboards[0].Title)
	assert.Equal(t, int64(25), dashboards[0].Revision)
	assert.Equal(t, int64(22), dashboards[0].ImportedRevision)
	assert.Equal(t, "db/nginx-connections", dashboards[0].ImportedUri)

	assert.Equal(t, int64(2), dashboards[1].Revision)
	assert.Equal(t, int64(0), dashboards[1].ImportedRevision)
}
