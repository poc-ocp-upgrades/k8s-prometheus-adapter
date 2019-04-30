package config

import (
	"fmt"
	"io/ioutil"
	"os"
	yaml "gopkg.in/yaml.v2"
)

func FromFile(filename string) (*MetricsDiscoveryConfig, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	file, err := os.Open(filename)
	defer file.Close()
	if err != nil {
		return nil, fmt.Errorf("unable to load metrics discovery config file: %v", err)
	}
	contents, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("unable to load metrics discovery config file: %v", err)
	}
	return FromYAML(contents)
}
func FromYAML(contents []byte) (*MetricsDiscoveryConfig, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	var cfg MetricsDiscoveryConfig
	if err := yaml.UnmarshalStrict(contents, &cfg); err != nil {
		return nil, fmt.Errorf("unable to parse metrics discovery config: %v", err)
	}
	return &cfg, nil
}
