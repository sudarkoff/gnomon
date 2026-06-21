package adminhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sudarkoff/gnomon"
)

// fakeEngine is a happy-path engine: Query and Series always succeed.
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

// errEngine simulates different error conditions.
type errEngine struct {
	queryErr  error
	seriesErr error
}

func (errEngine) Metrics() []gnomon.Metric { return nil }
func (e errEngine) Query(_ context.Context, name string) ([]gnomon.Row, error) {
	return nil, e.queryErr
}
func (e errEngine) Series(_ context.Context, name string, _, _ time.Time) ([]gnomon.Point, error) {
	return nil, e.seriesErr
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

// TestMetricEndpointDeepPath verifies that /metric/foo/bar returns 404 (does
// not match the {name} wildcard which only matches a single path segment).
func TestMetricEndpointDeepPath(t *testing.T) {
	srv := httptest.NewServer(NewHandler(fakeEngine{}))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/metric/foo/bar")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 for /metric/foo/bar, got %d", resp.StatusCode)
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

// TestMetricEndpointErrorCodes verifies that error types map to the correct
// HTTP status codes in the /metric/{name} handler.
func TestMetricEndpointErrorCodes(t *testing.T) {
	unknownErr := fmt.Errorf("%w: %q", gnomon.ErrUnknownMetric, "gone")
	backingErr := errors.New("db connection refused")

	cases := []struct {
		name       string
		engine     Engine
		wantStatus int
	}{
		{
			name:       "unknown metric -> 404",
			engine:     errEngine{queryErr: unknownErr},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "backing store error -> 500",
			engine:     errEngine{queryErr: backingErr},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(NewHandler(tc.engine))
			defer srv.Close()
			resp, err := http.Get(srv.URL + "/metric/whatever")
			if err != nil {
				t.Fatalf("request error: %v", err)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("want status %d, got %d", tc.wantStatus, resp.StatusCode)
			}
		})
	}
}

// TestSeriesEndpointErrorCodes verifies error mapping in the /series handler.
func TestSeriesEndpointErrorCodes(t *testing.T) {
	unknownErr := fmt.Errorf("%w: %q", gnomon.ErrUnknownMetric, "gone")
	backingErr := errors.New("db connection refused")

	cases := []struct {
		name       string
		engine     Engine
		wantStatus int
	}{
		{
			name:       "unknown metric -> 404",
			engine:     errEngine{seriesErr: unknownErr},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "backing store error -> 500",
			engine:     errEngine{seriesErr: backingErr},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(NewHandler(tc.engine))
			defer srv.Close()
			resp, err := http.Get(srv.URL + "/series?metric=whatever")
			if err != nil {
				t.Fatalf("request error: %v", err)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("want status %d, got %d", tc.wantStatus, resp.StatusCode)
			}
		})
	}
}

// declaredEngine returns a read-through metric that declares a dimension,
// measures, and the funnel chart type.
type declaredEngine struct{}

func (declaredEngine) Metrics() []gnomon.Metric {
	return []gnomon.Metric{{
		Name: "funnel", Title: "Acquisition funnel", Kind: gnomon.ReadThrough,
		Unit: gnomon.Count, Chart: gnomon.Funnel, Dimension: "signup_source",
		Measures: []gnomon.Measure{
			{Name: "signups", Label: "Signups", Unit: gnomon.Count},
			{Name: "paying", Label: "Paying", Unit: gnomon.Count},
		},
	}}
}
func (declaredEngine) Query(_ context.Context, _ string) ([]gnomon.Row, error) {
	return []gnomon.Row{{"signups": 10.0, "paying": 2.0}}, nil
}
func (declaredEngine) Series(_ context.Context, _ string, _, _ time.Time) ([]gnomon.Point, error) {
	return nil, nil
}

func TestMetricsEndpointDeclaredShape(t *testing.T) {
	srv := httptest.NewServer(NewHandler(declaredEngine{}))
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/metrics")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var out []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out) != 1 {
		t.Fatalf("want 1 metric, got %+v", out)
	}
	if out[0]["chart"] != "funnel" {
		t.Fatalf("chart = %v, want funnel", out[0]["chart"])
	}
	if out[0]["dimension"] != "signup_source" {
		t.Fatalf("dimension = %v, want signup_source", out[0]["dimension"])
	}
	ms, ok := out[0]["measures"].([]any)
	if !ok || len(ms) != 2 {
		t.Fatalf("measures = %v, want 2", out[0]["measures"])
	}
	m0 := ms[0].(map[string]any)
	if m0["name"] != "signups" || m0["label"] != "Signups" || m0["unit"] != "count" {
		t.Fatalf("measures[0] = %v", m0)
	}
}

// TestSeriesBadDate verifies that a malformed date param returns 400.
func TestSeriesBadDate(t *testing.T) {
	srv := httptest.NewServer(NewHandler(fakeEngine{}))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/series?metric=snap&from=notadate")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for bad date, got %d", resp.StatusCode)
	}
}
