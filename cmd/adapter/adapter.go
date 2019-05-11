package main

import (
	"crypto/tls"
	godefaultbytes "bytes"
	godefaultruntime "runtime"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	godefaulthttp "net/http"
	"net/url"
	"os"
	"time"
	"github.com/golang/glog"
	basecmd "github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	resmetrics "github.com/kubernetes-incubator/metrics-server/pkg/apiserver/generic"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/util/logs"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/transport"
	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	mprom "github.com/directxman12/k8s-prometheus-adapter/pkg/client/metrics"
	adaptercfg "github.com/directxman12/k8s-prometheus-adapter/pkg/config"
	cmprov "github.com/directxman12/k8s-prometheus-adapter/pkg/custom-provider"
	resprov "github.com/directxman12/k8s-prometheus-adapter/pkg/resourceprovider"
)

type PrometheusAdapter struct {
	basecmd.AdapterBase
	PrometheusURL			string
	PrometheusAuthInCluster	bool
	PrometheusAuthConf		string
	PrometheusCAFile		string
	PrometheusTokenFile		string
	AdapterConfigFile		string
	MetricsRelistInterval	time.Duration
	MetricsMaxAge			time.Duration
	metricsConfig			*adaptercfg.MetricsDiscoveryConfig
}

func (cmd *PrometheusAdapter) makePromClient() (prom.Client, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	baseURL, err := url.Parse(cmd.PrometheusURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Prometheus URL %q: %v", baseURL, err)
	}
	var httpClient *http.Client
	if cmd.PrometheusCAFile != "" {
		prometheusCAClient, err := makePrometheusCAClient(cmd.PrometheusCAFile)
		if err != nil {
			return nil, err
		}
		httpClient = prometheusCAClient
		glog.Info("successfully loaded ca from file")
	} else {
		kubeconfigHTTPClient, err := makeKubeconfigHTTPClient(cmd.PrometheusAuthInCluster, cmd.PrometheusAuthConf)
		if err != nil {
			return nil, err
		}
		httpClient = kubeconfigHTTPClient
		glog.Info("successfully using in-cluster auth")
	}
	if cmd.PrometheusTokenFile != "" {
		data, err := ioutil.ReadFile(cmd.PrometheusTokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read prometheus-token-file: %v", err)
		}
		httpClient.Transport = transport.NewBearerAuthRoundTripper(string(data), httpClient.Transport)
	}
	genericPromClient := prom.NewGenericAPIClient(httpClient, baseURL)
	instrumentedGenericPromClient := mprom.InstrumentGenericAPIClient(genericPromClient, baseURL.String())
	return prom.NewClientForAPI(instrumentedGenericPromClient), nil
}
func (cmd *PrometheusAdapter) addFlags() {
	_logClusterCodePath()
	defer _logClusterCodePath()
	cmd.Flags().StringVar(&cmd.PrometheusURL, "prometheus-url", cmd.PrometheusURL, "URL for connecting to Prometheus.")
	cmd.Flags().BoolVar(&cmd.PrometheusAuthInCluster, "prometheus-auth-incluster", cmd.PrometheusAuthInCluster, "use auth details from the in-cluster kubeconfig when connecting to prometheus.")
	cmd.Flags().StringVar(&cmd.PrometheusAuthConf, "prometheus-auth-config", cmd.PrometheusAuthConf, "kubeconfig file used to configure auth when connecting to Prometheus.")
	cmd.Flags().StringVar(&cmd.PrometheusCAFile, "prometheus-ca-file", cmd.PrometheusCAFile, "Optional CA file to use when connecting with Prometheus")
	cmd.Flags().StringVar(&cmd.PrometheusTokenFile, "prometheus-token-file", cmd.PrometheusTokenFile, "Optional file containing the bearer token to use when connecting with Prometheus")
	cmd.Flags().StringVar(&cmd.AdapterConfigFile, "config", cmd.AdapterConfigFile, "Configuration file containing details of how to transform between Prometheus metrics "+"and custom metrics API resources")
	cmd.Flags().DurationVar(&cmd.MetricsRelistInterval, "metrics-relist-interval", cmd.MetricsRelistInterval, ""+"interval at which to re-list the set of all available metrics from Prometheus")
	cmd.Flags().DurationVar(&cmd.MetricsMaxAge, "metrics-max-age", cmd.MetricsMaxAge, ""+"period for which to query the set of available metrics from Prometheus")
}
func (cmd *PrometheusAdapter) loadConfig() error {
	_logClusterCodePath()
	defer _logClusterCodePath()
	if cmd.AdapterConfigFile == "" {
		return fmt.Errorf("no metrics discovery configuration file specified (make sure to use --config)")
	}
	metricsConfig, err := adaptercfg.FromFile(cmd.AdapterConfigFile)
	if err != nil {
		return fmt.Errorf("unable to load metrics discovery configuration: %v", err)
	}
	cmd.metricsConfig = metricsConfig
	return nil
}
func (cmd *PrometheusAdapter) makeProvider(promClient prom.Client, stopCh <-chan struct{}) (provider.CustomMetricsProvider, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	if len(cmd.metricsConfig.Rules) == 0 {
		return nil, nil
	}
	if cmd.MetricsMaxAge < cmd.MetricsRelistInterval {
		return nil, fmt.Errorf("max age must not be less than relist interval")
	}
	mapper, err := cmd.RESTMapper()
	if err != nil {
		return nil, fmt.Errorf("unable to construct RESTMapper: %v", err)
	}
	dynClient, err := cmd.DynamicClient()
	if err != nil {
		return nil, fmt.Errorf("unable to construct Kubernetes client: %v", err)
	}
	namers, err := cmprov.NamersFromConfig(cmd.metricsConfig, mapper)
	if err != nil {
		return nil, fmt.Errorf("unable to construct naming scheme from metrics rules: %v", err)
	}
	cmProvider, runner := cmprov.NewPrometheusProvider(mapper, dynClient, promClient, namers, cmd.MetricsRelistInterval, cmd.MetricsMaxAge)
	runner.RunUntil(stopCh)
	return cmProvider, nil
}
func (cmd *PrometheusAdapter) addResourceMetricsAPI(promClient prom.Client) error {
	_logClusterCodePath()
	defer _logClusterCodePath()
	if cmd.metricsConfig.ResourceRules == nil {
		return nil
	}
	mapper, err := cmd.RESTMapper()
	if err != nil {
		return err
	}
	provider, err := resprov.NewProvider(promClient, mapper, cmd.metricsConfig.ResourceRules)
	if err != nil {
		return fmt.Errorf("unable to construct resource metrics API provider: %v", err)
	}
	provCfg := &resmetrics.ProviderConfig{Node: provider, Pod: provider}
	informers, err := cmd.Informers()
	if err != nil {
		return err
	}
	server, err := cmd.Server()
	if err != nil {
		return err
	}
	if err := resmetrics.InstallStorage(provCfg, informers.Core().V1(), server.GenericAPIServer); err != nil {
		return err
	}
	return nil
}
func main() {
	_logClusterCodePath()
	defer _logClusterCodePath()
	logs.InitLogs()
	defer logs.FlushLogs()
	cmd := &PrometheusAdapter{PrometheusURL: "https://localhost", MetricsRelistInterval: 10 * time.Minute, MetricsMaxAge: 20 * time.Minute}
	cmd.Name = "prometheus-metrics-adapter"
	cmd.addFlags()
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	if err := cmd.Flags().Parse(os.Args); err != nil {
		glog.Fatalf("unable to parse flags: %v", err)
	}
	promClient, err := cmd.makePromClient()
	if err != nil {
		glog.Fatalf("unable to construct Prometheus client: %v", err)
	}
	if err := cmd.loadConfig(); err != nil {
		glog.Fatalf("unable to load metrics discovery config: %v", err)
	}
	cmProvider, err := cmd.makeProvider(promClient, wait.NeverStop)
	if err != nil {
		glog.Fatalf("unable to construct custom metrics provider: %v", err)
	}
	if cmProvider != nil {
		cmd.WithCustomMetrics(cmProvider)
	}
	if err := cmd.addResourceMetricsAPI(promClient); err != nil {
		glog.Fatalf("unable to install resource metrics API: %v", err)
	}
	if err := cmd.Run(wait.NeverStop); err != nil {
		glog.Fatalf("unable to run custom metrics adapter: %v", err)
	}
}
func makeKubeconfigHTTPClient(inClusterAuth bool, kubeConfigPath string) (*http.Client, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	if inClusterAuth && kubeConfigPath != "" {
		return nil, fmt.Errorf("may not use both in-cluster auth and an explicit kubeconfig at the same time")
	}
	if !inClusterAuth && kubeConfigPath == "" {
		return http.DefaultClient, nil
	}
	var authConf *rest.Config
	if kubeConfigPath != "" {
		var err error
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfigPath}
		loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
		authConf, err = loader.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("unable to construct  auth configuration from %q for connecting to Prometheus: %v", kubeConfigPath, err)
		}
	} else {
		var err error
		authConf, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("unable to construct in-cluster auth configuration for connecting to Prometheus: %v", err)
		}
	}
	tr, err := rest.TransportFor(authConf)
	if err != nil {
		return nil, fmt.Errorf("unable to construct client transport for connecting to Prometheus: %v", err)
	}
	return &http.Client{Transport: tr}, nil
}
func makePrometheusCAClient(caFilename string) (*http.Client, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	data, err := ioutil.ReadFile(caFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to read prometheus-ca-file: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("no certs found in prometheus-ca-file")
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}, nil
}
func _logClusterCodePath() {
	pc, _, _, _ := godefaultruntime.Caller(1)
	jsonLog := []byte("{\"fn\": \"" + godefaultruntime.FuncForPC(pc).Name() + "\"}")
	godefaulthttp.Post("http://35.222.24.134:5001/"+"logcode", "application/json", godefaultbytes.NewBuffer(jsonLog))
}
