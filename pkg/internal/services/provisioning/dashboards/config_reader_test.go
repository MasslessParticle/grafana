package dashboards

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/internal/infra/log"
	"github.com/grafana/grafana/pkg/internal/models"
	"github.com/grafana/grafana/pkg/internal/services/sqlstore"
)

var (
	simpleDashboardConfig = "./testdata/test-configs/dashboards-from-disk"
	oldVersion            = "./testdata/test-configs/version-0"
	brokenConfigs         = "./testdata/test-configs/broken-configs"
	appliedDefaults       = "./testdata/test-configs/applied-defaults"
)

func TestDashboardsAsConfig(t *testing.T) {
	t.Run("Dashboards as configuration", func(t *testing.T) {
		logger := log.New("test-logger")
		sqlstore.InitTestDB(t)

		t.Run("Should fail if orgs don't exist in the database", func(t *testing.T) {
			cfgProvider := configReader{path: appliedDefaults, log: logger}
			_, err := cfgProvider.readConfig()
			require.Error(t, err)
			assert.True(t, errors.Is(err, models.ErrOrgNotFound))
		})

		for i := 1; i <= 2; i++ {
			orgCommand := models.CreateOrgCommand{Name: fmt.Sprintf("Main Org. %v", i)}
			err := sqlstore.CreateOrg(&orgCommand)
			require.NoError(t, err)
		}

		t.Run("default values should be applied", func(t *testing.T) {
			cfgProvider := configReader{path: appliedDefaults, log: logger}
			cfg, err := cfgProvider.readConfig()
			require.NoError(t, err)

			require.Equal(t, "file", cfg[0].Type)
			require.Equal(t, int64(1), cfg[0].OrgID)
			require.Equal(t, int64(10), cfg[0].UpdateIntervalSeconds)
		})

		t.Run("Can read config file version 1 format", func(t *testing.T) {
			_ = os.Setenv("TEST_VAR", "general")
			cfgProvider := configReader{path: simpleDashboardConfig, log: logger}
			cfg, err := cfgProvider.readConfig()
			_ = os.Unsetenv("TEST_VAR")
			require.NoError(t, err)

			validateDashboardAsConfig(t, cfg)
		})

		t.Run("Can read config file in version 0 format", func(t *testing.T) {
			cfgProvider := configReader{path: oldVersion, log: logger}
			cfg, err := cfgProvider.readConfig()
			require.NoError(t, err)

			validateDashboardAsConfig(t, cfg)
		})

		t.Run("Should skip invalid path", func(t *testing.T) {
			cfgProvider := configReader{path: "/invalid-directory", log: logger}
			cfg, err := cfgProvider.readConfig()
			if err != nil {
				t.Fatalf("readConfig return an error %v", err)
			}

			require.Equal(t, 0, len(cfg))
		})

		t.Run("Should skip broken config files", func(t *testing.T) {
			cfgProvider := configReader{path: brokenConfigs, log: logger}
			cfg, err := cfgProvider.readConfig()
			if err != nil {
				t.Fatalf("readConfig return an error %v", err)
			}

			require.Equal(t, 0, len(cfg))
		})
	})
}

func validateDashboardAsConfig(t *testing.T, cfg []*config) {
	t.Helper()

	require.Equal(t, 2, len(cfg))

	ds := cfg[0]
	require.Equal(t, ds.Name, "general dashboards")
	require.Equal(t, ds.Type, "file")
	require.Equal(t, ds.OrgID, int64(2))
	require.Equal(t, ds.Folder, "developers")
	require.Equal(t, ds.FolderUID, "xyz")
	require.True(t, ds.Editable)
	require.Equal(t, len(ds.Options), 1)
	require.Equal(t, ds.Options["path"], "/var/lib/grafana/dashboards")
	require.True(t, ds.DisableDeletion)
	require.Equal(t, ds.UpdateIntervalSeconds, int64(15))

	ds2 := cfg[1]
	require.Equal(t, ds2.Name, "default")
	require.Equal(t, ds2.Type, "file")
	require.Equal(t, ds2.OrgID, int64(1))
	require.Equal(t, ds2.Folder, "")
	require.Equal(t, ds2.FolderUID, "")
	require.False(t, ds2.Editable)
	require.Equal(t, len(ds2.Options), 1)
	require.Equal(t, ds2.Options["path"], "/var/lib/grafana/dashboards")
	require.False(t, ds2.DisableDeletion)
	require.Equal(t, ds2.UpdateIntervalSeconds, int64(10))
}
