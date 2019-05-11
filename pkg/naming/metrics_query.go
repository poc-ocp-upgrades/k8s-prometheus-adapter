package naming

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"k8s.io/apimachinery/pkg/runtime/schema"
	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

type MetricsQuery interface {
	Build(series string, groupRes schema.GroupResource, namespace string, extraGroupBy []string, resourceNames ...string) (prom.Selector, error)
}

func NewMetricsQuery(queryTemplate string, resourceConverter ResourceConverter) (MetricsQuery, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	templ, err := template.New("metrics-query").Delims("<<", ">>").Parse(queryTemplate)
	if err != nil {
		return nil, fmt.Errorf("unable to parse metrics query template %q: %v", queryTemplate, err)
	}
	return &metricsQuery{resConverter: resourceConverter, template: templ}, nil
}

type metricsQuery struct {
	resConverter	ResourceConverter
	template		*template.Template
}
type queryTemplateArgs struct {
	Series				string
	LabelMatchers		string
	LabelValuesByName	map[string][]string
	GroupBy				string
	GroupBySlice		[]string
}

func (q *metricsQuery) Build(series string, resource schema.GroupResource, namespace string, extraGroupBy []string, names ...string) (prom.Selector, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	var exprs []string
	valuesByName := map[string][]string{}
	if namespace != "" {
		namespaceLbl, err := q.resConverter.LabelForResource(nsGroupResource)
		if err != nil {
			return "", err
		}
		exprs = append(exprs, prom.LabelEq(string(namespaceLbl), namespace))
		valuesByName[string(namespaceLbl)] = []string{namespace}
	}
	resourceLbl, err := q.resConverter.LabelForResource(resource)
	if err != nil {
		return "", err
	}
	matcher := prom.LabelEq
	targetValue := names[0]
	if len(names) > 1 {
		matcher = prom.LabelMatches
		targetValue = strings.Join(names, "|")
	}
	exprs = append(exprs, matcher(string(resourceLbl), targetValue))
	valuesByName[string(resourceLbl)] = names
	groupBy := make([]string, 0, len(extraGroupBy)+1)
	groupBy = append(groupBy, string(resourceLbl))
	groupBy = append(groupBy, extraGroupBy...)
	args := queryTemplateArgs{Series: series, LabelMatchers: strings.Join(exprs, ","), LabelValuesByName: valuesByName, GroupBy: strings.Join(groupBy, ","), GroupBySlice: groupBy}
	queryBuff := new(bytes.Buffer)
	if err := q.template.Execute(queryBuff, args); err != nil {
		return "", err
	}
	if queryBuff.Len() == 0 {
		return "", fmt.Errorf("empty query produced by metrics query template")
	}
	return prom.Selector(queryBuff.String()), nil
}
