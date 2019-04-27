package provider

import (
	"context"
	"fmt"
	"time"
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider/helpers"
	pmodel "github.com/prometheus/common/model"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

type Runnable interface {
	Run()
	RunUntil(stopChan <-chan struct{})
}
type prometheusProvider struct {
	mapper		apimeta.RESTMapper
	kubeClient	dynamic.Interface
	promClient	prom.Client
	SeriesRegistry
}

func NewPrometheusProvider(mapper apimeta.RESTMapper, kubeClient dynamic.Interface, promClient prom.Client, namers []MetricNamer, updateInterval time.Duration, maxAge time.Duration) (provider.CustomMetricsProvider, Runnable) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	lister := &cachingMetricsLister{updateInterval: updateInterval, maxAge: maxAge, promClient: promClient, namers: namers, SeriesRegistry: &basicSeriesRegistry{mapper: mapper}}
	return &prometheusProvider{mapper: mapper, kubeClient: kubeClient, promClient: promClient, SeriesRegistry: lister}, lister
}
func (p *prometheusProvider) metricFor(value pmodel.SampleValue, name types.NamespacedName, info provider.CustomMetricInfo) (*custom_metrics.MetricValue, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	ref, err := helpers.ReferenceFor(p.mapper, name, info)
	if err != nil {
		return nil, err
	}
	return &custom_metrics.MetricValue{DescribedObject: ref, MetricName: info.Metric, Timestamp: metav1.Time{time.Now()}, Value: *resource.NewMilliQuantity(int64(value*1000.0), resource.DecimalSI)}, nil
}
func (p *prometheusProvider) metricsFor(valueSet pmodel.Vector, info provider.CustomMetricInfo, namespace string, names []string) (*custom_metrics.MetricValueList, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	values, found := p.MatchValuesToNames(info, valueSet)
	if !found {
		return nil, provider.NewMetricNotFoundError(info.GroupResource, info.Metric)
	}
	res := []custom_metrics.MetricValue{}
	for _, name := range names {
		if _, found := values[name]; !found {
			continue
		}
		value, err := p.metricFor(values[name], types.NamespacedName{Namespace: namespace, Name: name}, info)
		if err != nil {
			return nil, err
		}
		res = append(res, *value)
	}
	return &custom_metrics.MetricValueList{Items: res}, nil
}
func (p *prometheusProvider) buildQuery(info provider.CustomMetricInfo, namespace string, names ...string) (pmodel.Vector, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	query, found := p.QueryForMetric(info, namespace, names...)
	if !found {
		return nil, provider.NewMetricNotFoundError(info.GroupResource, info.Metric)
	}
	queryResults, err := p.promClient.Query(context.TODO(), pmodel.Now(), query)
	if err != nil {
		glog.Errorf("unable to fetch metrics from prometheus: %v", err)
		return nil, apierr.NewInternalError(fmt.Errorf("unable to fetch metrics"))
	}
	if queryResults.Type != pmodel.ValVector {
		glog.Errorf("unexpected results from prometheus: expected %s, got %s on results %v", pmodel.ValVector, queryResults.Type, queryResults)
		return nil, apierr.NewInternalError(fmt.Errorf("unable to fetch metrics"))
	}
	return *queryResults.Vector, nil
}
func (p *prometheusProvider) GetMetricByName(name types.NamespacedName, info provider.CustomMetricInfo) (*custom_metrics.MetricValue, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	queryResults, err := p.buildQuery(info, name.Namespace, name.Name)
	if err != nil {
		return nil, err
	}
	if len(queryResults) < 1 {
		return nil, provider.NewMetricNotFoundForError(info.GroupResource, info.Metric, name.Name)
	}
	namedValues, found := p.MatchValuesToNames(info, queryResults)
	if !found {
		return nil, provider.NewMetricNotFoundError(info.GroupResource, info.Metric)
	}
	if len(namedValues) > 1 {
		glog.V(2).Infof("Got more than one result (%v results) when fetching metric %s for %q, using the first one with a matching name...", len(queryResults), info.String(), name)
	}
	resultValue, nameFound := namedValues[name.Name]
	if !nameFound {
		glog.Errorf("None of the results returned by when fetching metric %s for %q matched the resource name", info.String(), name)
		return nil, provider.NewMetricNotFoundForError(info.GroupResource, info.Metric, name.Name)
	}
	return p.metricFor(resultValue, name, info)
}
func (p *prometheusProvider) GetMetricBySelector(namespace string, selector labels.Selector, info provider.CustomMetricInfo) (*custom_metrics.MetricValueList, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	resourceNames, err := helpers.ListObjectNames(p.mapper, p.kubeClient, namespace, selector, info)
	if err != nil {
		glog.Errorf("unable to list matching resource names: %v", err)
		return nil, apierr.NewInternalError(fmt.Errorf("unable to list matching resources"))
	}
	queryResults, err := p.buildQuery(info, namespace, resourceNames...)
	if err != nil {
		return nil, err
	}
	return p.metricsFor(queryResults, info, namespace, resourceNames)
}

type cachingMetricsLister struct {
	SeriesRegistry
	promClient	prom.Client
	updateInterval	time.Duration
	maxAge		time.Duration
	namers		[]MetricNamer
}

func (l *cachingMetricsLister) Run() {
	_logClusterCodePath()
	defer _logClusterCodePath()
	l.RunUntil(wait.NeverStop)
}
func (l *cachingMetricsLister) RunUntil(stopChan <-chan struct{}) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	go wait.Until(func() {
		if err := l.updateMetrics(); err != nil {
			utilruntime.HandleError(err)
		}
	}, l.updateInterval, stopChan)
}

type selectorSeries struct {
	selector	prom.Selector
	series		[]prom.Series
}

func (l *cachingMetricsLister) updateMetrics() error {
	_logClusterCodePath()
	defer _logClusterCodePath()
	startTime := pmodel.Now().Add(-1 * l.maxAge)
	seriesCacheByQuery := make(map[prom.Selector][]prom.Series)
	selectors := make(map[prom.Selector]struct{})
	selectorSeriesChan := make(chan selectorSeries, len(l.namers))
	errs := make(chan error, len(l.namers))
	for _, namer := range l.namers {
		sel := namer.Selector()
		if _, ok := selectors[sel]; ok {
			errs <- nil
			selectorSeriesChan <- selectorSeries{}
			continue
		}
		selectors[sel] = struct{}{}
		go func() {
			series, err := l.promClient.Series(context.TODO(), pmodel.Interval{startTime, 0}, sel)
			if err != nil {
				errs <- fmt.Errorf("unable to fetch metrics for query %q: %v", sel, err)
				return
			}
			errs <- nil
			selectorSeriesChan <- selectorSeries{selector: sel, series: series}
		}()
	}
	for range l.namers {
		if err := <-errs; err != nil {
			return fmt.Errorf("unable to update list of all metrics: %v", err)
		}
		if ss := <-selectorSeriesChan; ss.series != nil {
			seriesCacheByQuery[ss.selector] = ss.series
		}
	}
	close(errs)
	newSeries := make([][]prom.Series, len(l.namers))
	for i, namer := range l.namers {
		series, cached := seriesCacheByQuery[namer.Selector()]
		if !cached {
			return fmt.Errorf("unable to update list of all metrics: no metrics retrieved for query %q", namer.Selector())
		}
		newSeries[i] = namer.FilterSeries(series)
	}
	glog.V(10).Infof("Set available metric list from Prometheus to: %v", newSeries)
	return l.SetSeries(newSeries, l.namers)
}
