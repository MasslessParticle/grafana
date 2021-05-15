package channels

import (
	"context"
	"fmt"
	"path"

	gokit_log "github.com/go-kit/kit/log"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"

	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/services/alerting"
	old_notifiers "github.com/grafana/grafana/pkg/services/alerting/notifiers"
)

// KafkaNotifier is responsible for sending
// alert notifications to Kafka.
type KafkaNotifier struct {
	old_notifiers.NotifierBase
	Endpoint string
	Topic    string
	log      log.Logger
	tmpl     *template.Template
}

// NewKafkaNotifier is the constructor function for the Kafka notifier.
func NewKafkaNotifier(model *models.AlertNotification, t *template.Template) (*KafkaNotifier, error) {
	endpoint := model.Settings.Get("kafkaRestProxy").MustString()
	if endpoint == "" {
		return nil, alerting.ValidationError{Reason: "Could not find kafka rest proxy endpoint property in settings"}
	}
	topic := model.Settings.Get("kafkaTopic").MustString()
	if topic == "" {
		return nil, alerting.ValidationError{Reason: "Could not find kafka topic property in settings"}
	}

	return &KafkaNotifier{
		NotifierBase: old_notifiers.NewNotifierBase(model),
		Endpoint:     endpoint,
		Topic:        topic,
		log:          log.New("alerting.notifier.kafka"),
		tmpl:         t,
	}, nil
}

// Notify sends the alert notification.
func (kn *KafkaNotifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	// We are using the state from 7.x to not break kafka.
	// TODO: should we switch to the new ones?
	alerts := types.Alerts(as...)
	state := models.AlertStateAlerting
	if alerts.Status() == model.AlertResolved {
		state = models.AlertStateOK
	}

	kn.log.Debug("Notifying Kafka", "alert_state", state)

	data := notify.GetTemplateData(ctx, kn.tmpl, as, gokit_log.NewNopLogger())
	var tmplErr error
	tmpl := notify.TmplText(kn.tmpl, data, &tmplErr)

	bodyJSON := simplejson.New()
	bodyJSON.Set("alert_state", state)
	bodyJSON.Set("description", tmpl(`{{ template "default.title" . }}`))
	bodyJSON.Set("client", "Grafana")
	bodyJSON.Set("details", tmpl(`{{ template "default.message" . }}`))
	bodyJSON.Set("client_url", path.Join(kn.tmpl.ExternalURL.String(), "/alerting/list"))

	groupKey, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}
	bodyJSON.Set("incident_key", groupKey.Hash())

	valueJSON := simplejson.New()
	valueJSON.Set("value", bodyJSON)

	recordJSON := simplejson.New()
	recordJSON.Set("records", []interface{}{valueJSON})

	if tmplErr != nil {
		return false, fmt.Errorf("failed to template Kafka message: %w", tmplErr)
	}

	body, err := recordJSON.MarshalJSON()
	if err != nil {
		return false, err
	}

	topicURL := kn.Endpoint + "/topics/" + kn.Topic

	cmd := &models.SendWebhookSync{
		Url:        topicURL,
		Body:       string(body),
		HttpMethod: "POST",
		HttpHeader: map[string]string{
			"Content-Type": "application/vnd.kafka.json.v2+json",
			"Accept":       "application/vnd.kafka.v2+json",
		},
	}

	if err := bus.DispatchCtx(ctx, cmd); err != nil {
		kn.log.Error("Failed to send notification to Kafka", "error", err, "body", string(body))
		return false, err
	}

	return true, nil
}

func (kn *KafkaNotifier) SendResolved() bool {
	return !kn.GetDisableResolveMessage()
}
