package alerting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/grafana/grafana/pkg/internal/bus"
	"github.com/grafana/grafana/pkg/internal/models"
	apimodels "github.com/grafana/grafana/pkg/internal/services/ngalert/api/tooling/definitions"
	ngmodels "github.com/grafana/grafana/pkg/internal/services/ngalert/models"
	"github.com/grafana/grafana/pkg/internal/tests/testinfra"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlertRulePermissions(t *testing.T) {
	// Setup Grafana and its Database
	dir, path := testinfra.CreateGrafDir(t, testinfra.GrafanaOpts{
		EnableFeatureToggles: []string{"ngalert"},
		DisableAnonymous:     true,
	})
	store := testinfra.SetUpDatabase(t, dir)
	// override bus to get the GetSignedInUserQuery handler
	store.Bus = bus.GetBus()
	grafanaListedAddr := testinfra.StartGrafana(t, dir, path, store)

	// Create a user to make authenticated requests
	require.NoError(t, createUser(t, store, models.ROLE_EDITOR, "grafana", "password"))

	// Create the namespace we'll save our alerts to.
	require.NoError(t, createFolder(t, store, 0, "folder1"))

	// Create the namespace we'll save our alerts to.
	require.NoError(t, createFolder(t, store, 0, "folder2"))

	// Create rule under folder1
	createRule(t, grafanaListedAddr, "folder1")

	// Create rule under folder2
	createRule(t, grafanaListedAddr, "folder2")

	// With the rules created, let's make sure that rule definitions are stored.
	{
		u := fmt.Sprintf("http://grafana:password@%s/api/ruler/grafana/api/v1/rules", grafanaListedAddr)
		// nolint:gosec
		resp, err := http.Get(u)
		require.NoError(t, err)
		t.Cleanup(func() {
			err := resp.Body.Close()
			require.NoError(t, err)
		})
		b, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, resp.StatusCode, 202)

		body, _ := rulesNamespaceWithoutVariableValues(t, b)
		expectedGetNamespaceResponseBody := `
		{
		   "folder1":[
			  {
				 "name":"arulegroup",
				 "interval":"1m",
				 "rules":[
					{
						"annotations": {
							"annotation1": "val1"
					   },
					   "expr":"",
					   "for": "2m",
					   "labels": {
							"label1": "val1"
					   },
					   "grafana_alert":{
						  "id":1,
						  "orgId":1,
						  "title":"rule under folder folder1",
						  "condition":"A",
						  "data":[
		                     {
		                        "refId":"A",
		                        "queryType":"",
		                        "relativeTimeRange":{
		                           "from":18000,
		                           "to":10800
		                        },
		                        "datasourceUid":"-100",
								"model":{
		                           "expression":"2 + 3 \u003E 1",
		                           "intervalMs":1000,
		                           "maxDataPoints":43200,
		                           "type":"math"
		                        }
		                     }
		                  ],
						  "updated":"2021-02-21T01:10:30Z",
						  "intervalSeconds":60,
						  "version":1,
						  "uid":"uid",
						  "namespace_uid":"nsuid",
						  "namespace_id":1,
						  "rule_group":"arulegroup",
						  "no_data_state":"NoData",
						  "exec_err_state":"Alerting"
					   }
					}
				 ]
			  }
		   ],
		"folder2":[
			{
			   "name":"arulegroup",
			   "interval":"1m",
			   "rules":[
				  {
					  "annotations": {
						  "annotation1": "val1"
					 },
					 "expr":"",
					 "for": "2m",
					 "labels": {
						  "label1": "val1"
					 },
					 "grafana_alert":{
						"id":2,
						"orgId":1,
						"title":"rule under folder folder2",
						"condition":"A",
						"data":[
		                     {
		                        "refId":"A",
		                        "queryType":"",
		                        "relativeTimeRange":{
		                           "from":18000,
		                           "to":10800
		                        },
		                        "datasourceUid":"-100",
								"model":{
		                           "expression":"2 + 3 \u003E 1",
		                           "intervalMs":1000,
		                           "maxDataPoints":43200,
		                           "type":"math"
		                        }
		                     }
		                  ],
						"updated":"2021-02-21T01:10:30Z",
						"intervalSeconds":60,
						"version":1,
						"uid":"uid",
						"namespace_uid":"nsuid",
						"namespace_id":2,
						"rule_group":"arulegroup",
						"no_data_state":"NoData",
						"exec_err_state":"Alerting"
					 }
				  }
			   ]
			}
		 ]
		}`
		assert.JSONEq(t, expectedGetNamespaceResponseBody, body)

		// remove permissions from folder2
		require.NoError(t, store.UpdateDashboardACL(2, nil))

		// make sure that folder2 is not included in the response
		// nolint:gosec
		resp, err = http.Get(u)
		require.NoError(t, err)
		t.Cleanup(func() {
			err := resp.Body.Close()
			require.NoError(t, err)
		})
		b, err = ioutil.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, resp.StatusCode, 202)

		body, _ = rulesNamespaceWithoutVariableValues(t, b)
		expectedGetNamespaceResponseBody = `
		{
		   "folder1":[
			  {
				 "name":"arulegroup",
				 "interval":"1m",
				 "rules":[
					{
						"annotations": {
							"annotation1": "val1"
					   },
					   "expr":"",
					   "for": "2m",
					   "labels": {
							"label1": "val1"
					   },
					   "grafana_alert":{
						  "id":1,
						  "orgId":1,
						  "title":"rule under folder folder1",
						  "condition":"A",
						  "data":[
		                     {
		                        "refId":"A",
		                        "queryType":"",
		                        "relativeTimeRange":{
		                           "from":18000,
		                           "to":10800
		                        },
		                        "datasourceUid":"-100",
								"model":{
		                           "expression":"2 + 3 \u003E 1",
		                           "intervalMs":1000,
		                           "maxDataPoints":43200,
		                           "type":"math"
		                        }
		                     }
		                  ],
						  "updated":"2021-02-21T01:10:30Z",
						  "intervalSeconds":60,
						  "version":1,
						  "uid":"uid",
						  "namespace_uid":"nsuid",
						  "namespace_id":1,
						  "rule_group":"arulegroup",
						  "no_data_state":"NoData",
						  "exec_err_state":"Alerting"
					   }
					}
				 ]
			  }
		   ]
		}`
		assert.JSONEq(t, expectedGetNamespaceResponseBody, body)
	}
}

func createRule(t *testing.T, grafanaListedAddr string, folder string) {
	t.Helper()

	interval, err := model.ParseDuration("1m")
	require.NoError(t, err)

	rules := apimodels.PostableRuleGroupConfig{
		Name:     "arulegroup",
		Interval: interval,
		Rules: []apimodels.PostableExtendedRuleNode{
			{
				ApiRuleNode: &apimodels.ApiRuleNode{
					For:         2 * interval,
					Labels:      map[string]string{"label1": "val1"},
					Annotations: map[string]string{"annotation1": "val1"},
				},
				GrafanaManagedAlert: &apimodels.PostableGrafanaRule{
					Title:     fmt.Sprintf("rule under folder %s", folder),
					Condition: "A",
					Data: []ngmodels.AlertQuery{
						{
							RefID: "A",
							RelativeTimeRange: ngmodels.RelativeTimeRange{
								From: ngmodels.Duration(time.Duration(5) * time.Hour),
								To:   ngmodels.Duration(time.Duration(3) * time.Hour),
							},
							DatasourceUID: "-100",
							Model: json.RawMessage(`{
								"type": "math",
								"expression": "2 + 3 > 1"
								}`),
						},
					},
				},
			},
		},
	}
	buf := bytes.Buffer{}
	enc := json.NewEncoder(&buf)
	err = enc.Encode(&rules)
	require.NoError(t, err)

	u := fmt.Sprintf("http://grafana:password@%s/api/ruler/grafana/api/v1/rules/%s", grafanaListedAddr, folder)
	// nolint:gosec
	resp, err := http.Post(u, "application/json", &buf)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := resp.Body.Close()
		require.NoError(t, err)
	})
	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	require.JSONEq(t, `{"message":"rule group updated successfully"}`, string(b))
}
