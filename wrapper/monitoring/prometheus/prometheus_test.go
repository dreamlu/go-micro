package prometheus_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/dreamlu/go-micro/v2/broker"
	bmemory "github.com/dreamlu/go-micro/v2/broker/memory"
	"github.com/dreamlu/go-micro/v2/client"
	"github.com/dreamlu/go-micro/v2/client/selector"
	"github.com/dreamlu/go-micro/v2/registry/memory"
	"github.com/dreamlu/go-micro/v2/server"
	promwrapper "github.com/dreamlu/go-plugins/wrapper/monitoring/prometheus/v2"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

type Test interface {
	Method(ctx context.Context, in *TestRequest, opts ...client.CallOption) (*TestResponse, error)
}

type TestRequest struct {
	IsError bool
}
type TestResponse struct{}

type testHandler struct{}

func (t *testHandler) Method(ctx context.Context, req *TestRequest, rsp *TestResponse) error {
	if req.IsError {
		return fmt.Errorf("test error")
	}
	return nil
}

func TestPrometheusMetrics(t *testing.T) {
	// setup
	reg := memory.NewRegistry()
	brk := bmemory.NewBroker(broker.Registry(reg))
	sel := selector.NewSelector(selector.Registry(reg))

	name := "test"
	id := "id-1234567890"
	version := "1.2.3.4"

	md := make(map[string]string)
	md["dc"] = "dc1"
	md["node"] = "node1"

	c := client.NewClient(client.Selector(sel))
	s := server.NewServer(
		server.Name(name),
		server.Version(version),
		server.Id(id),
		server.Registry(reg),
		server.Broker(brk),
		server.WrapHandler(
			promwrapper.NewHandlerWrapper(
				server.Metadata(md),
				server.Name(name),
				server.Version(version),
				server.Id(id),
			),
		),
	)

	defer s.Stop()

	type Test struct {
		*testHandler
	}

	s.Handle(
		s.NewHandler(&Test{new(testHandler)}),
	)

	if err := s.Start(); err != nil {
		t.Fatalf("Unexpected error starting server: %v", err)
	}

	req := c.NewRequest(name, "Test.Method", &TestRequest{IsError: false}, client.WithContentType("application/json"))
	rsp := TestResponse{}

	assert.NoError(t, c.Call(context.TODO(), req, &rsp))

	req = c.NewRequest(name, "Test.Method", &TestRequest{IsError: true}, client.WithContentType("application/json"))
	assert.Error(t, c.Call(context.TODO(), req, &rsp))

	list, _ := prometheus.DefaultGatherer.Gather()

	metric := findMetricByName(list, dto.MetricType_SUMMARY, "micro_handler_latency_microseconds")

	if metric == nil || metric.Metric == nil || len(metric.Metric) == 0 {
		t.Fatalf("no metrics returned")
	}

	for _, v := range metric.Metric[0].Label {
		switch *v.Name {
		case "micro_dc":
			assert.Equal(t, "dc1", *v.Value)
		case "micro_node":
			assert.Equal(t, "node1", *v.Value)
		case "micro_version":
			assert.Equal(t, version, *v.Value)
		case "micro_id":
			assert.Equal(t, id, *v.Value)
		case "micro_name":
			assert.Equal(t, name, *v.Value)
		case "method":
			assert.Equal(t, "Test.Method", *v.Value)
		default:
			t.Fatalf("unknown %v with %v", *v.Name, *v.Value)
		}
	}

	assert.Equal(t, uint64(2), *metric.Metric[0].Summary.SampleCount)
	assert.True(t, *metric.Metric[0].Summary.SampleSum > 0)

	metric = findMetricByName(list, dto.MetricType_HISTOGRAM, "micro_handler_request_duration_seconds")

	for _, v := range metric.Metric[0].Label {
		switch *v.Name {
		case "micro_dc":
			assert.Equal(t, "dc1", *v.Value)
		case "micro_node":
			assert.Equal(t, "node1", *v.Value)
		case "micro_version":
			assert.Equal(t, version, *v.Value)
		case "micro_id":
			assert.Equal(t, id, *v.Value)
		case "micro_name":
			assert.Equal(t, name, *v.Value)
		case "method":
			assert.Equal(t, "Test.Method", *v.Value)
		default:
			t.Fatalf("unknown %v with %v", *v.Name, *v.Value)
		}
	}

	assert.Equal(t, uint64(2), *metric.Metric[0].Histogram.SampleCount)
	assert.True(t, *metric.Metric[0].Histogram.SampleSum > 0)

	metric = findMetricByName(list, dto.MetricType_COUNTER, "micro_handler_request_total")

	for _, v := range metric.Metric[0].Label {
		switch *v.Name {
		case "micro_dc":
			assert.Equal(t, "dc1", *v.Value)
		case "micro_node":
			assert.Equal(t, "node1", *v.Value)
		case "micro_version":
			assert.Equal(t, version, *v.Value)
		case "micro_id":
			assert.Equal(t, id, *v.Value)
		case "micro_name":
			assert.Equal(t, name, *v.Value)
		case "method":
			assert.Equal(t, "Test.Method", *v.Value)
		case "status":
			assert.Equal(t, "fail", *v.Value)
		}
	}
	assert.Equal(t, *metric.Metric[0].Counter.Value, float64(1))

	for _, v := range metric.Metric[1].Label {
		switch *v.Name {
		case "dc":
			assert.Equal(t, "dc1", *v.Value)
		case "node":
			assert.Equal(t, "node1", *v.Value)
		case "micro_version":
			assert.Equal(t, version, *v.Value)
		case "micro_id":
			assert.Equal(t, id, *v.Value)
		case "micro_name":
			assert.Equal(t, name, *v.Value)
		case "method":
			assert.Equal(t, "Test.Method", *v.Value)
		case "status":
			assert.Equal(t, "success", *v.Value)
		}
	}

	assert.Equal(t, *metric.Metric[1].Counter.Value, float64(1))
}

func findMetricByName(list []*dto.MetricFamily, tp dto.MetricType, name string) *dto.MetricFamily {
	for _, metric := range list {
		if *metric.Name == name && *metric.Type == tp {
			return metric
		}
	}

	return nil
}