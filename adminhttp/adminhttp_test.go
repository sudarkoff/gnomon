package adminhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sudarkoff/gnomon"
)

type fakeEngine struct{}

func (fakeEngine) Metrics() []gnomon.Metric {
	return []gnomon.Metric{{Name: "mrr", Title: "MRR", Kind: gnomon.ReadThrough, Unit: gnomon.Cents, Chart: gnomon.Stat}}
}
func (fakeEngine) Query(_ context.Context, name string) ([]gnomon.Row, error) {
	return []gnomon.Row{{"value": 100.0}}, nil
}
func (fakeEngine) Series(_ context.Context, name string, _, _ time.Time) ([]gnomon.Point, error) {
	return []gnomon.Point{{Dimension: "", Value: 5}}, nil
}

func TestMetricsEndpoint(t *testing.T) {
	srv := httptest.NewServer(NewHandler(fakeEngine{}))
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/metrics")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var out []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out) != 1 || out[0]["name"] != "mrr" || out[0]["kind"] != "read_through" {
		t.Fatalf("bad metrics payload: %+v", out)
	}
}

func TestMetricEndpoint(t *testing.T) {
	srv := httptest.NewServer(NewHandler(fakeEngine{}))
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/metric/mrr")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var out []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out) != 1 || out[0]["value"] != 100.0 {
		t.Fatalf("bad metric payload: %+v", out)
	}
}

func TestSeriesEndpoint(t *testing.T) {
	srv := httptest.NewServer(NewHandler(fakeEngine{}))
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/series?metric=snap")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var out []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out) != 1 || out[0]["value"] != 5.0 {
		t.Fatalf("bad series payload: %+v", out)
	}
}
