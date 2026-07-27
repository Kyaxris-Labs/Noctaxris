package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	cwsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cloudwatch"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	cwJSONContentType = "application/x-amz-json-1.0"
	cwEventSource     = "monitoring.amazonaws.com"
)

func (s *Server) handleCloudWatch(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = cloudWatchAction(action)

	switch action {
	case catalog.ActionCloudWatchPutMetricData:
		s.cwPutMetricData(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudWatchListMetrics:
		s.cwListMetrics(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudWatchGetMetricStatistics:
		s.cwGetMetricStatistics(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudWatchGetMetricData:
		s.cwGetMetricData(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudWatchPutMetricAlarm:
		s.cwPutMetricAlarm(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudWatchDescribeAlarms:
		s.cwDescribeAlarms(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudWatchDeleteAlarms:
		s.cwDeleteAlarms(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudWatchSetAlarmState:
		s.cwSetAlarmState(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeCWError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This CloudWatch Metrics action is not implemented.", readOnly, eventID, verified)
	}
}

func cloudWatchAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "PutMetricData":
		return catalog.ActionCloudWatchPutMetricData
	case "ListMetrics":
		return catalog.ActionCloudWatchListMetrics
	case "GetMetricStatistics":
		return catalog.ActionCloudWatchGetMetricStatistics
	case "GetMetricData":
		return catalog.ActionCloudWatchGetMetricData
	case "PutMetricAlarm":
		return catalog.ActionCloudWatchPutMetricAlarm
	case "DescribeAlarms":
		return catalog.ActionCloudWatchDescribeAlarms
	case "DeleteAlarms":
		return catalog.ActionCloudWatchDeleteAlarms
	case "SetAlarmState":
		return catalog.ActionCloudWatchSetAlarmState
	default:
		return action
	}
}

func parseCWDimensions(v any) []store.CWDimension {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []store.CWDimension
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["Name"].(string)
		val, _ := m["Value"].(string)
		if name == "" {
			continue
		}
		out = append(out, store.CWDimension{Name: name, Value: val})
	}
	return out
}

func parseCWFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func parseCWInt(v any) (int, bool) {
	f, ok := parseCWFloat(v)
	if !ok {
		return 0, false
	}
	return int(f), true
}

func parseCWTimestamp(v any) int64 {
	switch t := v.(type) {
	case float64:
		// seconds or millis
		if t > 1e12 {
			return int64(t / 1000)
		}
		return int64(t)
	case int64:
		if t > 1e12 {
			return t / 1000
		}
		return t
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			if f > 1e12 {
				return int64(f / 1000)
			}
			return int64(f)
		}
		if tm, err := time.Parse(time.RFC3339, s); err == nil {
			return tm.UTC().Unix()
		}
		if tm, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return tm.UTC().Unix()
		}
	}
	return 0
}

func parseCWMetricData(v any) []store.CWMetricDatum {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []store.CWMetricDatum
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		d := store.CWMetricDatum{}
		d.MetricName, _ = m["MetricName"].(string)
		d.Unit, _ = m["Unit"].(string)
		d.Dimensions = parseCWDimensions(m["Dimensions"])
		d.Timestamp = parseCWTimestamp(m["Timestamp"])
		if val, ok := parseCWFloat(m["Value"]); ok {
			d.Value = val
		}
		if stats, ok := m["StatisticValues"].(map[string]any); ok {
			d.HasStats = true
			if sc, ok := parseCWFloat(stats["SampleCount"]); ok {
				d.SampleCount = sc
			}
			if sum, ok := parseCWFloat(stats["Sum"]); ok {
				d.Sum = sum
			}
			if mn, ok := parseCWFloat(stats["Minimum"]); ok {
				d.Minimum = mn
			}
			if mx, ok := parseCWFloat(stats["Maximum"]); ok {
				d.Maximum = mx
			}
		}
		out = append(out, d)
	}
	return out
}

func parseCWStringList(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range list {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (s *Server) cwPutMetricData(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudWatchPutMetricData, "*") {
		s.writeCWError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudwatch:PutMetricData.", readOnly, eventID, verified)
		return
	}
	ns, _ := params["Namespace"].(string)
	datums := parseCWMetricData(params["MetricData"])
	err := s.store.PutMetricData(verified.AccountID, verified.Region, ns, datums)
	if errors.Is(err, store.ErrCWMissingParameter) || errors.Is(err, store.ErrCWBadRequest) {
		s.writeCWError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCWError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceError",
			"Unable to put metric data.", readOnly, eventID, verified)
		return
	}
	payload, _ := cwsvc.PutMetricDataJSON()
	s.writeCWOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cwEventSource, "PutMetricData", readOnly)
}

func (s *Server) cwListMetrics(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudWatchListMetrics, "*") {
		s.writeCWError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudwatch:ListMetrics.", readOnly, eventID, verified)
		return
	}
	ns, _ := params["Namespace"].(string)
	mn, _ := params["MetricName"].(string)
	dims := parseCWDimensions(params["Dimensions"])
	metrics, err := s.store.ListMetrics(verified.AccountID, verified.Region, ns, mn, dims)
	if err != nil {
		s.writeCWError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceError",
			"Unable to list metrics.", readOnly, eventID, verified)
		return
	}
	payload, _ := cwsvc.ListMetricsJSON(metrics)
	s.writeCWOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cwEventSource, "ListMetrics", readOnly)
}

func (s *Server) cwGetMetricStatistics(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudWatchGetMetricStatistics, "*") {
		s.writeCWError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudwatch:GetMetricStatistics.", readOnly, eventID, verified)
		return
	}
	ns, _ := params["Namespace"].(string)
	mn, _ := params["MetricName"].(string)
	unit, _ := params["Unit"].(string)
	period, _ := parseCWInt(params["Period"])
	start := parseCWTimestamp(params["StartTime"])
	end := parseCWTimestamp(params["EndTime"])
	dims := parseCWDimensions(params["Dimensions"])
	stats := parseCWStringList(params["Statistics"])
	dps, err := s.store.GetMetricStatistics(verified.AccountID, verified.Region, ns, mn, dims, start, end, period, unit)
	if errors.Is(err, store.ErrCWMissingParameter) || errors.Is(err, store.ErrCWBadRequest) {
		s.writeCWError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCWError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceError",
			"Unable to get metric statistics.", readOnly, eventID, verified)
		return
	}
	payload, _ := cwsvc.GetMetricStatisticsJSON(mn, stats, dps)
	s.writeCWOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cwEventSource, "GetMetricStatistics", readOnly)
}

func (s *Server) cwGetMetricData(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudWatchGetMetricData, "*") {
		s.writeCWError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudwatch:GetMetricData.", readOnly, eventID, verified)
		return
	}
	start := parseCWTimestamp(params["StartTime"])
	end := parseCWTimestamp(params["EndTime"])
	var queries []store.CWMetricDataQuery
	if list, ok := params["MetricDataQueries"].([]any); ok {
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			q := store.CWMetricDataQuery{ReturnData: true}
			q.ID, _ = m["Id"].(string)
			q.Label, _ = m["Label"].(string)
			if rd, ok := m["ReturnData"].(bool); ok {
				q.ReturnData = rd
			}
			if ms, ok := m["MetricStat"].(map[string]any); ok {
				st := &store.CWMetricStat{}
				if metric, ok := ms["Metric"].(map[string]any); ok {
					st.Namespace, _ = metric["Namespace"].(string)
					st.MetricName, _ = metric["MetricName"].(string)
					st.Dimensions = parseCWDimensions(metric["Dimensions"])
				}
				st.Stat, _ = ms["Stat"].(string)
				st.Unit, _ = ms["Unit"].(string)
				if p, ok := parseCWInt(ms["Period"]); ok {
					st.Period = p
				}
				q.MetricStat = st
			}
			queries = append(queries, q)
		}
	}
	results, err := s.store.GetCloudWatchMetricData(verified.AccountID, verified.Region, queries, start, end)
	if err != nil {
		s.writeCWError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceError",
			"Unable to get metric data.", readOnly, eventID, verified)
		return
	}
	payload, _ := cwsvc.GetMetricDataJSON(results)
	s.writeCWOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cwEventSource, "GetMetricData", readOnly)
}

func (s *Server) cwPutMetricAlarm(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudWatchPutMetricAlarm, "*") {
		s.writeCWError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudwatch:PutMetricAlarm.", readOnly, eventID, verified)
		return
	}
	alarm := store.CWMetricAlarm{ActionsEnabled: true}
	alarm.AlarmName, _ = params["AlarmName"].(string)
	alarm.AlarmDescription, _ = params["AlarmDescription"].(string)
	alarm.MetricName, _ = params["MetricName"].(string)
	alarm.Namespace, _ = params["Namespace"].(string)
	alarm.Statistic, _ = params["Statistic"].(string)
	alarm.ComparisonOperator, _ = params["ComparisonOperator"].(string)
	alarm.Dimensions = parseCWDimensions(params["Dimensions"])
	if p, ok := parseCWInt(params["Period"]); ok {
		alarm.Period = p
	}
	if ep, ok := parseCWInt(params["EvaluationPeriods"]); ok {
		alarm.EvaluationPeriods = ep
	}
	if th, ok := parseCWFloat(params["Threshold"]); ok {
		alarm.Threshold = th
	}
	if ae, ok := params["ActionsEnabled"].(bool); ok {
		alarm.ActionsEnabled = ae
	}
	_, err := s.store.PutMetricAlarm(verified.AccountID, verified.Region, alarm)
	if errors.Is(err, store.ErrCWMissingParameter) || errors.Is(err, store.ErrCWBadRequest) {
		s.writeCWError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCWError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceError",
			"Unable to put metric alarm.", readOnly, eventID, verified)
		return
	}
	payload, _ := cwsvc.PutMetricAlarmJSON()
	s.writeCWOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cwEventSource, "PutMetricAlarm", readOnly)
}

func (s *Server) cwDescribeAlarms(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudWatchDescribeAlarms, "*") {
		s.writeCWError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudwatch:DescribeAlarms.", readOnly, eventID, verified)
		return
	}
	names := parseCWStringList(params["AlarmNames"])
	prefix, _ := params["AlarmNamePrefix"].(string)
	alarms, err := s.store.DescribeAlarms(verified.AccountID, verified.Region, names, prefix)
	if err != nil {
		s.writeCWError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceError",
			"Unable to describe alarms.", readOnly, eventID, verified)
		return
	}
	payload, _ := cwsvc.DescribeAlarmsJSON(alarms)
	s.writeCWOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cwEventSource, "DescribeAlarms", readOnly)
}

func (s *Server) cwDeleteAlarms(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudWatchDeleteAlarms, "*") {
		s.writeCWError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudwatch:DeleteAlarms.", readOnly, eventID, verified)
		return
	}
	names := parseCWStringList(params["AlarmNames"])
	if err := s.store.DeleteAlarms(verified.AccountID, verified.Region, names); err != nil {
		s.writeCWError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceError",
			"Unable to delete alarms.", readOnly, eventID, verified)
		return
	}
	payload, _ := cwsvc.DeleteAlarmsJSON()
	s.writeCWOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cwEventSource, "DeleteAlarms", readOnly)
}

func (s *Server) cwSetAlarmState(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudWatchSetAlarmState, "*") {
		s.writeCWError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudwatch:SetAlarmState.", readOnly, eventID, verified)
		return
	}
	name, _ := params["AlarmName"].(string)
	state, _ := params["StateValue"].(string)
	reason, _ := params["StateReason"].(string)
	reasonData, _ := params["StateReasonData"].(string)
	err := s.store.SetAlarmState(verified.AccountID, verified.Region, name, state, reason, reasonData)
	if errors.Is(err, store.ErrCWAlarmNotFound) {
		s.writeCWError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFound",
			"Alarm not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCWMissingParameter) || errors.Is(err, store.ErrCWBadRequest) {
		s.writeCWError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCWError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceError",
			"Unable to set alarm state.", readOnly, eventID, verified)
		return
	}
	payload, _ := cwsvc.SetAlarmStateJSON()
	s.writeCWOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cwEventSource, "SetAlarmState", readOnly)
}

func (s *Server) writeCWOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", cwJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCWError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", cwJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, cwEventSource, code, readOnly)
}
