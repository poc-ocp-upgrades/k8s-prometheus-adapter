package provider

import (
	"fmt"
	"sync"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/golang/glog"
	pmodel "github.com/prometheus/common/model"
)

type SeriesType int

const (
	CounterSeries	SeriesType	= iota
	SecondsCounterSeries
	GaugeSeries
)

type SeriesRegistry interface {
	SetSeries(series [][]prom.Series, namers []MetricNamer) error
	ListAllMetrics() []provider.CustomMetricInfo
	QueryForMetric(info provider.CustomMetricInfo, namespace string, resourceNames ...string) (query prom.Selector, found bool)
	MatchValuesToNames(metricInfo provider.CustomMetricInfo, values pmodel.Vector) (matchedValues map[string]pmodel.SampleValue, found bool)
}
type seriesInfo struct {
	seriesName	string
	namer		MetricNamer
}
type basicSeriesRegistry struct {
	mu		sync.RWMutex
	info	map[provider.CustomMetricInfo]seriesInfo
	metrics	[]provider.CustomMetricInfo
	mapper	apimeta.RESTMapper
}

func (r *basicSeriesRegistry) SetSeries(newSeriesSlices [][]prom.Series, namers []MetricNamer) error {
	_logClusterCodePath()
	defer _logClusterCodePath()
	if len(newSeriesSlices) != len(namers) {
		return fmt.Errorf("need one set of series per namer")
	}
	newInfo := make(map[provider.CustomMetricInfo]seriesInfo)
	for i, newSeries := range newSeriesSlices {
		namer := namers[i]
		for _, series := range newSeries {
			resources, namespaced := namer.ResourcesForSeries(series)
			name, err := namer.MetricNameForSeries(series)
			if err != nil {
				glog.Errorf("unable to name series %q, skipping: %v", series.String(), err)
				continue
			}
			for _, resource := range resources {
				info := provider.CustomMetricInfo{GroupResource: resource, Namespaced: namespaced, Metric: name}
				if resource == nsGroupResource {
					info.Namespaced = false
				}
				newInfo[info] = seriesInfo{seriesName: series.Name, namer: namer}
			}
		}
	}
	newMetrics := make([]provider.CustomMetricInfo, 0, len(newInfo))
	for info := range newInfo {
		newMetrics = append(newMetrics, info)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.info = newInfo
	r.metrics = newMetrics
	return nil
}
func (r *basicSeriesRegistry) ListAllMetrics() []provider.CustomMetricInfo {
	_logClusterCodePath()
	defer _logClusterCodePath()
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.metrics
}
func (r *basicSeriesRegistry) QueryForMetric(metricInfo provider.CustomMetricInfo, namespace string, resourceNames ...string) (prom.Selector, bool) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(resourceNames) == 0 {
		glog.Errorf("no resource names requested while producing a query for metric %s", metricInfo.String())
		return "", false
	}
	metricInfo, _, err := metricInfo.Normalized(r.mapper)
	if err != nil {
		glog.Errorf("unable to normalize group resource while producing a query: %v", err)
		return "", false
	}
	info, infoFound := r.info[metricInfo]
	if !infoFound {
		glog.V(10).Infof("metric %v not registered", metricInfo)
		return "", false
	}
	query, err := info.namer.QueryForSeries(info.seriesName, metricInfo.GroupResource, namespace, resourceNames...)
	if err != nil {
		glog.Errorf("unable to construct query for metric %s: %v", metricInfo.String(), err)
		return "", false
	}
	return query, true
}
func (r *basicSeriesRegistry) MatchValuesToNames(metricInfo provider.CustomMetricInfo, values pmodel.Vector) (matchedValues map[string]pmodel.SampleValue, found bool) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	r.mu.RLock()
	defer r.mu.RUnlock()
	metricInfo, _, err := metricInfo.Normalized(r.mapper)
	if err != nil {
		glog.Errorf("unable to normalize group resource while matching values to names: %v", err)
		return nil, false
	}
	info, infoFound := r.info[metricInfo]
	if !infoFound {
		return nil, false
	}
	resourceLbl, err := info.namer.LabelForResource(metricInfo.GroupResource)
	if err != nil {
		glog.Errorf("unable to construct resource label for metric %s: %v", metricInfo.String(), err)
		return nil, false
	}
	res := make(map[string]pmodel.SampleValue, len(values))
	for _, val := range values {
		if val == nil {
			continue
		}
		res[string(val.Metric[resourceLbl])] = val.Value
	}
	return res, true
}
