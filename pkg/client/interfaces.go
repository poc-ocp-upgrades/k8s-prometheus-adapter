package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"github.com/prometheus/common/model"
)

type Selector string
type Range struct {
	Start, End	model.Time
	Step		time.Duration
}
type Client interface {
	Series(ctx context.Context, interval model.Interval, selectors ...Selector) ([]Series, error)
	Query(ctx context.Context, t model.Time, query Selector) (QueryResult, error)
	QueryRange(ctx context.Context, r Range, query Selector) (QueryResult, error)
}
type QueryResult struct {
	Type	model.ValueType
	Vector	*model.Vector
	Scalar	*model.Scalar
	Matrix	*model.Matrix
}

func (qr *QueryResult) UnmarshalJSON(b []byte) error {
	_logClusterCodePath()
	defer _logClusterCodePath()
	v := struct {
		Type	model.ValueType	`json:"resultType"`
		Result	json.RawMessage	`json:"result"`
	}{}
	err := json.Unmarshal(b, &v)
	if err != nil {
		return err
	}
	qr.Type = v.Type
	switch v.Type {
	case model.ValScalar:
		var sv model.Scalar
		err = json.Unmarshal(v.Result, &sv)
		qr.Scalar = &sv
	case model.ValVector:
		var vv model.Vector
		err = json.Unmarshal(v.Result, &vv)
		qr.Vector = &vv
	case model.ValMatrix:
		var mv model.Matrix
		err = json.Unmarshal(v.Result, &mv)
		qr.Matrix = &mv
	default:
		err = fmt.Errorf("unexpected value type %q", v.Type)
	}
	return err
}

type Series struct {
	Name	string
	Labels	model.LabelSet
}

func (s *Series) UnmarshalJSON(data []byte) error {
	_logClusterCodePath()
	defer _logClusterCodePath()
	var rawMetric model.Metric
	err := json.Unmarshal(data, &rawMetric)
	if err != nil {
		return err
	}
	if name, ok := rawMetric[model.MetricNameLabel]; ok {
		s.Name = string(name)
		delete(rawMetric, model.MetricNameLabel)
	}
	s.Labels = model.LabelSet(rawMetric)
	return nil
}
func (s *Series) String() string {
	_logClusterCodePath()
	defer _logClusterCodePath()
	lblStrings := make([]string, 0, len(s.Labels))
	for k, v := range s.Labels {
		lblStrings = append(lblStrings, fmt.Sprintf("%s=%q", k, v))
	}
	return fmt.Sprintf("%s{%s}", s.Name, strings.Join(lblStrings, ","))
}
