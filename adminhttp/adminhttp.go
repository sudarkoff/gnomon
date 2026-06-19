// Package adminhttp exposes a mountable, auth-agnostic JSON handler for serving
// gnomon metric metadata, live ReadThrough results, and stored Snapshot series.
// Wrap it with your own auth middleware.
package adminhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sudarkoff/gnomon"
)

// Engine is the read surface adminhttp needs. *gnomon.Gnomon satisfies it.
type Engine interface {
	Metrics() []gnomon.Metric
	Query(ctx context.Context, name string) ([]gnomon.Row, error)
	Series(ctx context.Context, name string, from, to time.Time) ([]gnomon.Point, error)
}

type metricDTO struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	Unit  string `json:"unit"`
	Chart string `json:"chart"`
}

func kindStr(k gnomon.Kind) string {
	if k == gnomon.Snapshot {
		return "snapshot"
	}
	return "read_through"
}
func unitStr(u gnomon.Unit) string {
	switch u {
	case gnomon.Percent:
		return "percent"
	case gnomon.Cents:
		return "cents"
	default:
		return "count"
	}
}
func chartStr(c gnomon.Chart) string {
	switch c {
	case gnomon.Line:
		return "line"
	case gnomon.Bar:
		return "bar"
	default:
		return "stat"
	}
}

// NewHandler returns an http.Handler serving /metrics, /metric/{name}, /series.
func NewHandler(e Engine) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		out := make([]metricDTO, 0)
		for _, m := range e.Metrics() {
			out = append(out, metricDTO{m.Name, m.Title, kindStr(m.Kind), unitStr(m.Unit), chartStr(m.Chart)})
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("/metric/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/metric/")
		if name == "" {
			http.Error(w, "missing metric name", http.StatusBadRequest)
			return
		}
		rows, err := e.Query(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, rows)
	})

	mux.HandleFunc("/series", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("metric")
		if name == "" {
			http.Error(w, "missing metric param", http.StatusBadRequest)
			return
		}
		to := parseDay(r.URL.Query().Get("to"), time.Now().UTC())
		from := parseDay(r.URL.Query().Get("from"), to.AddDate(0, 0, -90))
		pts, err := e.Series(r.Context(), name, from, to)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if pts == nil {
			pts = []gnomon.Point{}
		}
		writeJSON(w, pts)
	})

	return mux
}

func parseDay(s string, def time.Time) time.Time {
	if s == "" {
		return def
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return def
	}
	return t
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
