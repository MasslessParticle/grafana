package channels

import (
	"context"
	"net/url"
	"testing"

	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/internal/bus"
	"github.com/grafana/grafana/pkg/internal/components/simplejson"
	"github.com/grafana/grafana/pkg/internal/models"
)

func TestEmailNotifier(t *testing.T) {
	tmpl := templateForTests(t)

	externalURL, err := url.Parse("http://localhost")
	require.NoError(t, err)
	tmpl.ExternalURL = externalURL

	t.Run("empty settings should return error", func(t *testing.T) {
		json := `{ }`

		settingsJSON, _ := simplejson.NewJson([]byte(json))
		model := &NotificationChannelConfig{
			Name:     "ops",
			Type:     "email",
			Settings: settingsJSON,
		}

		_, err := NewEmailNotifier(model, tmpl)
		require.Error(t, err)
	})

	t.Run("with the correct settings it should not fail and produce the expected command", func(t *testing.T) {
		json := `{
			"addresses": "someops@example.com;somedev@example.com",
			"message": "{{ template \"default.title\" . }}"
		}`
		settingsJSON, err := simplejson.NewJson([]byte(json))
		require.NoError(t, err)

		emailNotifier, err := NewEmailNotifier(&NotificationChannelConfig{
			Name:     "ops",
			Type:     "email",
			Settings: settingsJSON,
		}, tmpl)

		require.NoError(t, err)

		expected := map[string]interface{}{}
		bus.AddHandlerCtx("test", func(ctx context.Context, cmd *models.SendEmailCommandSync) error {
			expected["subject"] = cmd.SendEmailCommand.Subject
			expected["to"] = cmd.SendEmailCommand.To
			expected["single_email"] = cmd.SendEmailCommand.SingleEmail
			expected["template"] = cmd.SendEmailCommand.Template
			expected["data"] = cmd.SendEmailCommand.Data

			return nil
		})

		alerts := []*types.Alert{
			{
				Alert: model.Alert{
					Labels:      model.LabelSet{"alertname": "AlwaysFiring", "severity": "warning"},
					Annotations: model.LabelSet{"runbook_url": "http://fix.me"},
				},
			},
		}

		ok, err := emailNotifier.Notify(context.Background(), alerts...)
		require.NoError(t, err)
		require.True(t, ok)

		require.Equal(t, map[string]interface{}{
			"subject":      "[FIRING:1]  (AlwaysFiring warning)",
			"to":           []string{"someops@example.com", "somedev@example.com"},
			"single_email": false,
			"template":     "ng_alert_notification.html",
			"data": map[string]interface{}{
				"Title":   "[FIRING:1]  (AlwaysFiring warning)",
				"Message": "[FIRING:1]  (AlwaysFiring warning)",
				"Status":  "firing",
				"Alerts": template.Alerts{
					template.Alert{
						Status:      "firing",
						Labels:      template.KV{"alertname": "AlwaysFiring", "severity": "warning"},
						Annotations: template.KV{"runbook_url": "http://fix.me"},
						Fingerprint: "15a37193dce72bab",
					},
				},
				"GroupLabels":       template.KV{},
				"CommonLabels":      template.KV{"alertname": "AlwaysFiring", "severity": "warning"},
				"CommonAnnotations": template.KV{"runbook_url": "http://fix.me"},
				"ExternalURL":       "http://localhost",
				"RuleUrl":           "http:/localhost/alerting/list",
				"AlertPageUrl":      "http:/localhost/alerting/list?alertState=firing&view=state",
			},
		}, expected)
	})
}
