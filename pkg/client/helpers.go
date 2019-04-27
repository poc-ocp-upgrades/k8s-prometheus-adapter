package client

import (
	"fmt"
	"strings"
)

func LabelNeq(label string, value string) string {
	_logClusterCodePath()
	defer _logClusterCodePath()
	_logClusterCodePath()
	defer _logClusterCodePath()
	return fmt.Sprintf("%s!=%q", label, value)
}
func LabelEq(label string, value string) string {
	_logClusterCodePath()
	defer _logClusterCodePath()
	_logClusterCodePath()
	defer _logClusterCodePath()
	return fmt.Sprintf("%s=%q", label, value)
}
func LabelMatches(label string, expr string) string {
	_logClusterCodePath()
	defer _logClusterCodePath()
	_logClusterCodePath()
	defer _logClusterCodePath()
	return fmt.Sprintf("%s=~%q", label, expr)
}
func LabelNotMatches(label string, expr string) string {
	_logClusterCodePath()
	defer _logClusterCodePath()
	_logClusterCodePath()
	defer _logClusterCodePath()
	return fmt.Sprintf("%s!~%q", label, expr)
}
func NameMatches(expr string) string {
	_logClusterCodePath()
	defer _logClusterCodePath()
	_logClusterCodePath()
	defer _logClusterCodePath()
	return LabelMatches("__name__", expr)
}
func NameNotMatches(expr string) string {
	_logClusterCodePath()
	defer _logClusterCodePath()
	_logClusterCodePath()
	defer _logClusterCodePath()
	return LabelNotMatches("__name__", expr)
}
func MatchSeries(name string, labelExpressions ...string) Selector {
	_logClusterCodePath()
	defer _logClusterCodePath()
	_logClusterCodePath()
	defer _logClusterCodePath()
	if len(labelExpressions) == 0 {
		return Selector(name)
	}
	return Selector(fmt.Sprintf("%s{%s}", name, strings.Join(labelExpressions, ",")))
}
