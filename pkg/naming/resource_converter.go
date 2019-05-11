package naming

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"text/template"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	pmodel "github.com/prometheus/common/model"
)

var (
	groupNameSanitizer	= strings.NewReplacer(".", "_", "-", "_")
	nsGroupResource		= schema.GroupResource{Resource: "namespaces"}
)

type ResourceConverter interface {
	ResourcesForSeries(series prom.Series) (res []schema.GroupResource, namespaced bool)
	LabelForResource(resource schema.GroupResource) (pmodel.LabelName, error)
}
type resourceConverter struct {
	labelResourceMu		sync.RWMutex
	labelToResource		map[pmodel.LabelName]schema.GroupResource
	resourceToLabel		map[schema.GroupResource]pmodel.LabelName
	labelResExtractor	*labelGroupResExtractor
	mapper				apimeta.RESTMapper
	labelTemplate		*template.Template
}

func NewResourceConverter(resourceTemplate string, overrides map[string]config.GroupResource, mapper apimeta.RESTMapper) (ResourceConverter, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	converter := &resourceConverter{labelToResource: make(map[pmodel.LabelName]schema.GroupResource), resourceToLabel: make(map[schema.GroupResource]pmodel.LabelName), mapper: mapper}
	if resourceTemplate != "" {
		labelTemplate, err := template.New("resource-label").Delims("<<", ">>").Parse(resourceTemplate)
		if err != nil {
			return converter, fmt.Errorf("unable to parse label template %q: %v", resourceTemplate, err)
		}
		converter.labelTemplate = labelTemplate
		labelResExtractor, err := newLabelGroupResExtractor(labelTemplate)
		if err != nil {
			return converter, fmt.Errorf("unable to generate label format from template %q: %v", resourceTemplate, err)
		}
		converter.labelResExtractor = labelResExtractor
	}
	for lbl, groupRes := range overrides {
		infoRaw := provider.CustomMetricInfo{GroupResource: schema.GroupResource{Group: groupRes.Group, Resource: groupRes.Resource}}
		info, _, err := infoRaw.Normalized(converter.mapper)
		if err != nil {
			return nil, fmt.Errorf("unable to normalize group-resource %v: %v", groupRes, err)
		}
		converter.labelToResource[pmodel.LabelName(lbl)] = info.GroupResource
		converter.resourceToLabel[info.GroupResource] = pmodel.LabelName(lbl)
	}
	return converter, nil
}
func (r *resourceConverter) LabelForResource(resource schema.GroupResource) (pmodel.LabelName, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	r.labelResourceMu.RLock()
	lbl, ok := r.resourceToLabel[resource]
	r.labelResourceMu.RUnlock()
	if ok {
		return lbl, nil
	}
	lbl, err := r.makeLabelForResource(resource)
	if err != nil {
		return "", fmt.Errorf("unable to convert resource %s into label: %v", resource.String(), err)
	}
	return lbl, nil
}
func (r *resourceConverter) makeLabelForResource(resource schema.GroupResource) (pmodel.LabelName, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	if r.labelTemplate == nil {
		return "", fmt.Errorf("no generic resource label form specified for this metric")
	}
	buff := new(bytes.Buffer)
	singularRes, err := r.mapper.ResourceSingularizer(resource.Resource)
	if err != nil {
		return "", fmt.Errorf("unable to singularize resource %s: %v", resource.String(), err)
	}
	convResource := schema.GroupResource{Group: groupNameSanitizer.Replace(resource.Group), Resource: singularRes}
	if err := r.labelTemplate.Execute(buff, convResource); err != nil {
		return "", err
	}
	if buff.Len() == 0 {
		return "", fmt.Errorf("empty label produced by label template")
	}
	lbl := pmodel.LabelName(buff.String())
	r.labelResourceMu.Lock()
	defer r.labelResourceMu.Unlock()
	r.resourceToLabel[resource] = lbl
	r.labelToResource[lbl] = resource
	return lbl, nil
}
func (r *resourceConverter) ResourcesForSeries(series prom.Series) ([]schema.GroupResource, bool) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	var resources []schema.GroupResource
	updates := make(map[pmodel.LabelName]schema.GroupResource)
	namespaced := false
	func() {
		r.labelResourceMu.RLock()
		defer r.labelResourceMu.RUnlock()
		for lbl := range series.Labels {
			var groupRes schema.GroupResource
			var ok bool
			if groupRes, ok = r.labelToResource[lbl]; ok {
				resources = append(resources, groupRes)
			} else if groupRes, ok = updates[lbl]; ok {
				resources = append(resources, groupRes)
			} else if r.labelResExtractor != nil {
				if groupRes, ok = r.labelResExtractor.GroupResourceForLabel(lbl); ok {
					info, _, err := provider.CustomMetricInfo{GroupResource: groupRes}.Normalized(r.mapper)
					if err != nil {
						glog.V(9).Infof("unable to normalize group-resource %s from label %q, skipping: %v", groupRes.String(), lbl, err)
						continue
					}
					groupRes = info.GroupResource
					resources = append(resources, groupRes)
					updates[lbl] = groupRes
				}
			}
			if groupRes == nsGroupResource {
				namespaced = true
			}
		}
	}()
	if len(updates) > 0 {
		r.labelResourceMu.Lock()
		defer r.labelResourceMu.Unlock()
		for lbl, groupRes := range updates {
			r.labelToResource[lbl] = groupRes
		}
	}
	return resources, namespaced
}
