# gnomon

A tiny, standardized BI layer for Go services — the metrics sibling to
[flagpole](https://github.com/sudarkoff/flagpole). Register business metrics,
then capture and serve them. No scheduler, no auth, no UI — those belong to the
host.

## Concepts

- **ReadThrough** metric: queried live on request.
- **Snapshot** metric: captured once per day into `gnomon_snapshots` for trends.

## Usage

```go
g := gnomon.New(sourcepg.New(pool), sourcepg.New(pool))
g.Register(
    gnomon.Metric{Name: "mrr", Title: "MRR", Kind: gnomon.ReadThrough,
        SQL: "SELECT * FROM analytics_mrr", Unit: gnomon.Cents, Chart: gnomon.Stat},
    gnomon.Metric{Name: "mrr_daily", Title: "MRR over time", Kind: gnomon.Snapshot,
        SQL: "SELECT mrr_usd::float8 AS value FROM analytics_mrr", Unit: gnomon.Cents, Chart: gnomon.Line},
)

// In a daily job:
_ = g.Capture(ctx, time.Now().UTC())

// Mount the JSON API behind your own auth:
mux.Handle("/admin/insights/", http.StripPrefix("/admin/insights", adminhttp.NewHandler(g)))
```

Run `sourcepg/schema.sql` via your own migration tooling.
