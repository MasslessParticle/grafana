package opentsdb

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	"golang.org/x/net/context/ctxhttp"

	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/grafana/grafana/pkg/internal/components/null"
	"github.com/grafana/grafana/pkg/internal/infra/log"
	"github.com/grafana/grafana/pkg/internal/models"
	"github.com/grafana/grafana/pkg/internal/plugins"
	"github.com/grafana/grafana/pkg/internal/setting"
)

type OpenTsdbExecutor struct {
}

// nolint:staticcheck // plugins.DataQueryResult deprecated
func NewExecutor(*models.DataSource) (plugins.DataPlugin, error) {
	return &OpenTsdbExecutor{}, nil
}

var (
	plog = log.New("tsdb.opentsdb")
)

// nolint:staticcheck // plugins.DataQueryResult deprecated
func (e *OpenTsdbExecutor) DataQuery(ctx context.Context, dsInfo *models.DataSource,
	queryContext plugins.DataQuery) (plugins.DataResponse, error) {
	var tsdbQuery OpenTsdbQuery

	tsdbQuery.Start = queryContext.TimeRange.GetFromAsMsEpoch()
	tsdbQuery.End = queryContext.TimeRange.GetToAsMsEpoch()

	for _, query := range queryContext.Queries {
		metric := e.buildMetric(query)
		tsdbQuery.Queries = append(tsdbQuery.Queries, metric)
	}

	// TODO: Don't use global variable
	if setting.Env == setting.Dev {
		plog.Debug("OpenTsdb request", "params", tsdbQuery)
	}

	req, err := e.createRequest(dsInfo, tsdbQuery)
	if err != nil {
		return plugins.DataResponse{}, err
	}

	httpClient, err := dsInfo.GetHttpClient()
	if err != nil {
		return plugins.DataResponse{}, err
	}

	res, err := ctxhttp.Do(ctx, httpClient, req)
	if err != nil {
		return plugins.DataResponse{}, err
	}

	queryResult, err := e.parseResponse(tsdbQuery, res)
	if err != nil {
		return plugins.DataResponse{}, err
	}

	return plugins.DataResponse{
		Results: queryResult,
	}, nil
}

func (e *OpenTsdbExecutor) createRequest(dsInfo *models.DataSource, data OpenTsdbQuery) (*http.Request, error) {
	u, err := url.Parse(dsInfo.Url)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, "api/query")

	postData, err := json.Marshal(data)
	if err != nil {
		plog.Info("Failed marshaling data", "error", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, u.String(), strings.NewReader(string(postData)))
	if err != nil {
		plog.Info("Failed to create request", "error", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if dsInfo.BasicAuth {
		req.SetBasicAuth(dsInfo.BasicAuthUser, dsInfo.DecryptedBasicAuthPassword())
	}

	return req, nil
}

// nolint:staticcheck // plugins.DataQueryResult deprecated
func (e *OpenTsdbExecutor) parseResponse(query OpenTsdbQuery, res *http.Response) (map[string]plugins.DataQueryResult, error) {
	queryResults := make(map[string]plugins.DataQueryResult)
	queryRes := plugins.DataQueryResult{}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			plog.Warn("Failed to close response body", "err", err)
		}
	}()

	if res.StatusCode/100 != 2 {
		plog.Info("Request failed", "status", res.Status, "body", string(body))
		return nil, fmt.Errorf("request failed, status: %s", res.Status)
	}

	var data []OpenTsdbResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		plog.Info("Failed to unmarshal opentsdb response", "error", err, "status", res.Status, "body", string(body))
		return nil, err
	}

	for _, val := range data {
		series := plugins.DataTimeSeries{
			Name: val.Metric,
		}

		for timeString, value := range val.DataPoints {
			timestamp, err := strconv.ParseFloat(timeString, 64)
			if err != nil {
				plog.Info("Failed to unmarshal opentsdb timestamp", "timestamp", timeString)
				return nil, err
			}
			series.Points = append(series.Points, plugins.DataTimePoint{
				null.FloatFrom(value), null.FloatFrom(timestamp),
			})
		}

		queryRes.Series = append(queryRes.Series, series)
	}

	queryResults["A"] = queryRes
	return queryResults, nil
}

func (e *OpenTsdbExecutor) buildMetric(query plugins.DataSubQuery) map[string]interface{} {
	metric := make(map[string]interface{})

	// Setting metric and aggregator
	metric["metric"] = query.Model.Get("metric").MustString()
	metric["aggregator"] = query.Model.Get("aggregator").MustString()

	// Setting downsampling options
	disableDownsampling := query.Model.Get("disableDownsampling").MustBool()
	if !disableDownsampling {
		downsampleInterval := query.Model.Get("downsampleInterval").MustString()
		if downsampleInterval == "" {
			downsampleInterval = "1m" // default value for blank
		}
		downsample := downsampleInterval + "-" + query.Model.Get("downsampleAggregator").MustString()
		if query.Model.Get("downsampleFillPolicy").MustString() != "none" {
			metric["downsample"] = downsample + "-" + query.Model.Get("downsampleFillPolicy").MustString()
		} else {
			metric["downsample"] = downsample
		}
	}

	// Setting rate options
	if query.Model.Get("shouldComputeRate").MustBool() {
		metric["rate"] = true
		rateOptions := make(map[string]interface{})
		rateOptions["counter"] = query.Model.Get("isCounter").MustBool()

		counterMax, counterMaxCheck := query.Model.CheckGet("counterMax")
		if counterMaxCheck {
			rateOptions["counterMax"] = counterMax.MustFloat64()
		}

		resetValue, resetValueCheck := query.Model.CheckGet("counterResetValue")
		if resetValueCheck {
			rateOptions["resetValue"] = resetValue.MustFloat64()
		}

		if !counterMaxCheck && (!resetValueCheck || resetValue.MustFloat64() == 0) {
			rateOptions["dropResets"] = true
		}

		metric["rateOptions"] = rateOptions
	}

	// Setting tags
	tags, tagsCheck := query.Model.CheckGet("tags")
	if tagsCheck && len(tags.MustMap()) > 0 {
		metric["tags"] = tags.MustMap()
	}

	// Setting filters
	filters, filtersCheck := query.Model.CheckGet("filters")
	if filtersCheck && len(filters.MustArray()) > 0 {
		metric["filters"] = filters.MustArray()
	}

	return metric
}
