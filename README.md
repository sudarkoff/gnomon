# gnomon

A tiny, standardized BI layer for Go services — the metrics sibling to
[flagpole](https://github.com/sudarkoff/flagpole).

You **register business metrics in code** — each one a name, a unit, a chart
type, and the SQL that computes it — and gnomon captures and serves them. Live
numbers (MRR, active users) are queried on demand; daily numbers are snapshotted
once a day so you can plot a trend. There is no scheduler, no auth, and no UI:
gnomon owns the metric registry and the math, and hands the rest back to the host
application, which wires `Capture` into its own job runner and wraps the HTTP
handler with its own auth.

```go
g := gnomon.New(sourcepg.New(pool), sourcepg.New(pool))
g.Register(
    gnomon.Metric{Name: "mrr", Title: "MRR", Kind: gnomon.ReadThrough,
        SQL: "SELECT mrr_cents AS value FROM analytics_mrr", Unit: gnomon.Cents, Chart: gnomon.Stat},
)

rows, _ := g.Query(ctx, "mrr")           // live, on request
```

---

## Contents

- [Why gnomon](#why-gnomon)
- [Install](#install)
- [Quick start](#quick-start)
- [Concepts](#concepts)
- [Engine API](#engine-api)
- [DataSource & Store](#datasource--store)
- [sourcepg — Postgres adapter](#sourcepg--postgres-adapter)
- [Admin HTTP handler](#admin-http-handler)
- [Experiment analysis](#experiment-analysis)
- [How it works](#how-it-works)
- [Testing](#testing)
- [Roadmap](#roadmap)
- [License](#license)

---

## Why gnomon

- **Metrics live in code, next to the service that owns them.** A metric is a
  Go value — `Metric{Name, Title, Kind, SQL, Unit, Chart, ...}` — registered at
  startup. No dashboards drawn by hand, no BI tool reaching into your database
  with its own credentials, no metric definitions drifting from the code that
  produced the data.
- **Two metric kinds, one registry.** *ReadThrough* metrics run their SQL live
  on request (current MRR, active users). *Snapshot* metrics run once a day and
  store the result so you can read back a trend over time. Both are declared the
  same way and served from the same surface.
- **No new infrastructure.** gnomon stores daily snapshots in one table in your
  existing Postgres. There is no separate metrics database, no daemon, and no
  sidecar — `Capture` is just a function you call from a job you already run.
- **You keep scheduling, auth, and UI.** gnomon is deliberately incomplete:
  it does not decide *when* to capture (wire it into your scheduler), *who* may
  read (wrap the handler in your auth), or *how* it's drawn (the handler emits
  JSON; the chart is yours). That's what keeps it tiny.
- **Honest about its scope.** The core engine is a registry plus four methods.
  The statistics live in a separate, dependency-free `experiments` package you
  can use on its own.

The core `gnomon` package depends only on the standard library plus
`pgx/v5/pgtype` (used to coerce assorted Postgres numeric types to `float64`).
The `sourcepg` adapter pulls in `pgx`; the `adminhttp` handler and the
`experiments` analysis package use the standard library only.

## Install

```
go get github.com/sudarkoff/gnomon
```

Requires Go 1.25+.

## Quick start

```go
import (
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/sudarkoff/gnomon"
    "github.com/sudarkoff/gnomon/adminhttp"
    "github.com/sudarkoff/gnomon/sourcepg"
)

pool, _ := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
pg := sourcepg.New(pool) // satisfies BOTH DataSource (runs metric SQL) and Store (snapshot history)

g := gnomon.New(pg, pg)
if err := g.Register(
    // Live: queried on every request, nothing stored.
    gnomon.Metric{
        Name: "mrr", Title: "MRR", Kind: gnomon.ReadThrough,
        SQL: "SELECT mrr_cents AS value FROM analytics_mrr",
        Unit: gnomon.Cents, Chart: gnomon.Stat,
    },
    // Snapshot: captured once a day into gnomon_snapshots for the trend line.
    gnomon.Metric{
        Name: "mrr_daily", Title: "MRR over time", Kind: gnomon.Snapshot,
        SQL: "SELECT mrr_cents::float8 AS value FROM analytics_mrr",
        Unit: gnomon.Cents, Chart: gnomon.Line,
    },
); err != nil {
    log.Fatal(err)
}

// In a daily job (you own the schedule):
if err := g.Capture(ctx, time.Now().UTC()); err != nil {
    log.Printf("gnomon capture: %v", err) // partial failures are joined, not fatal
}

// Mount the JSON API behind your own auth:
mux.Handle("/admin/insights/",
    requireAdmin(http.StripPrefix("/admin/insights", adminhttp.NewHandler(g))))
```

Run `sourcepg/schema.sql` through your own migration tooling to create the
`gnomon_snapshots` table.

## Concepts

### Metric

A **Metric** is one registered measure. It pairs a SQL query with the metadata
needed to capture, serve, and render it.

```go
type Metric struct {
    Name      string    // stable identifier, unique within the registry
    Title     string    // human label for display
    Kind      Kind      // ReadThrough or Snapshot
    SQL       string    // the query that computes the metric
    Unit      Unit      // Count, Percent, or Cents — how to format the value
    Chart     Chart     // Stat, Line, Bar, or Funnel — how to render it
    Dimension string    // (ReadThrough) the column to use as the x-axis / grouping key
    Measures  []Measure // (ReadThrough) the columns to plot/show, in order
}
```

### Kind: ReadThrough vs Snapshot

| Kind | When SQL runs | Stored? | Use for |
|------|---------------|---------|---------|
| `ReadThrough` | live, on each request via `Query` | no | current values, breakdowns, funnels |
| `Snapshot` | once per `Capture(ctx, day)` | yes, in `gnomon_snapshots` | trends over time read back via `Series` |

A **Snapshot** metric's SQL must return a `value` column (coerced to `float64`)
and may return a `dimension` column to store one row per series per day (e.g. one
point per plan tier). A **ReadThrough** metric's SQL may return arbitrary
columns; `Dimension` names the x-axis column and `Measures` names the columns to
plot.

### Unit & Chart

`Unit` is `Count`, `Percent`, or `Cents` — a formatting hint for whatever renders
the value. `Chart` is `Stat`, `Line`, `Bar`, or `Funnel`. gnomon never draws
anything; these travel through the JSON API so your frontend knows how to.

### Measure

For a ReadThrough metric that plots several series, each **Measure** names one
column to read, an optional display label, and the unit to format it with:

```go
type Measure struct {
    Name  string // result column to read
    Label string // optional display label
    Unit  Unit
}
```

## Engine API

`gnomon.New(data DataSource, store Store) *Gnomon` builds an engine over a data
source (runs metric SQL) and a snapshot store (persists/reads history). The two
can be the same object — `sourcepg.New(pool)` satisfies both.

| Method | Description |
|--------|-------------|
| `Register(ms ...Metric) error` | Validate and add metrics **atomically**: names must be unique (across the registry *and* within the batch) and non-empty, SQL non-empty. If any metric in the batch is invalid, none are added. |
| `Metrics() []Metric` | All registered metrics, in registration order. |
| `Capture(ctx, on time.Time) error` | Run every **Snapshot** metric's SQL, parse rows into samples, and upsert them stamped with `on`. ReadThrough metrics are skipped. Per-metric all-or-nothing; one metric failing does not abort the rest — all errors are joined. |
| `Query(ctx, name) ([]Row, error)` | Run a **ReadThrough** metric live and return its rows. |
| `Series(ctx, name, from, to) ([]Point, error)` | Read stored **Snapshot** history for a metric in `[from, to]`. |

`Query` and `Series` return sentinel errors you can match with `errors.Is`:

- `ErrUnknownMetric` — the name was never registered.
- `ErrWrongKind` — `Query` on a Snapshot metric, or `Series` on a ReadThrough
  metric.

`Capture` is idempotent per day: re-running it for the same `on` date overwrites
that day's samples (the store upserts on `(captured_on, metric, dimension)`), so
a retried or backfilled job is safe.

## DataSource & Store

gnomon talks to your database through two small interfaces. Implement them over
anything; `sourcepg` is the batteries-included Postgres implementation of both.

```go
// DataSource runs metric SQL against the host's data database.
type DataSource interface {
    Query(ctx context.Context, sql string) ([]Row, error)
}

// Store persists and reads snapshot history.
type Store interface {
    UpsertSnapshots(ctx context.Context, on time.Time, metric string, samples []Sample) error
    ReadSeries(ctx context.Context, metric string, from, to time.Time) ([]Point, error)
}
```

A `Row` is a `map[string]any` (one result row, column → value). A `Sample` is one
captured `{Dimension, Value}`; a `Point` is one `{CapturedOn, Dimension, Value}`
read back. Snapshot SQL is parsed by reading the `value` column (coerced from the
usual pgx numeric types to `float64`) and an optional `dimension` column.

> Metric SQL is executed verbatim. Treat metric definitions as **trusted,
> code-reviewed config**, not user input — never build a `Metric.SQL` string
> from untrusted data.

## sourcepg — Postgres adapter

`sourcepg.New(pool *pgxpool.Pool) *sourcepg.Store` returns one value that
satisfies **both** `gnomon.DataSource` and `gnomon.Store`:

- `Query` runs arbitrary metric SQL and collects rows as `column → value` maps.
- `UpsertSnapshots` writes a day's samples in a single batch, upserting on
  `(captured_on, metric, dimension)` (empty `samples` is a no-op).
- `ReadSeries` returns stored points for a metric within `[from, to]`, ordered by
  day then dimension.

Run the reference schema (`sourcepg/schema.sql`) with your own migration tooling:

```sql
CREATE TABLE IF NOT EXISTS gnomon_snapshots (
    captured_on  DATE             NOT NULL,
    metric       TEXT             NOT NULL,
    dimension    TEXT             NOT NULL DEFAULT '',
    value        DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (captured_on, metric, dimension)
);
```

> `ReadSeries` compares `from`/`to` against a `DATE` column in their own
> `time.Location`. Pass **UTC**-based times to avoid off-by-one-day boundary
> surprises — the same `time.Now().UTC()` you hand to `Capture`.

## Admin HTTP handler

`adminhttp.NewHandler(e Engine) http.Handler` is a mountable, **auth-agnostic**
JSON handler. `*gnomon.Gnomon` satisfies `Engine`. Wrap it with your own
authentication/authorization before mounting — gnomon ships no auth.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/metrics` | Metadata for every registered metric (name, title, kind, unit, chart, dimension, measures) as JSON. |
| `GET` | `/metric/{name}` | Live result rows for a **ReadThrough** metric. |
| `GET` | `/series?metric={name}&from=YYYY-MM-DD&to=YYYY-MM-DD` | Stored **Snapshot** history. `to` defaults to today (UTC); `from` defaults to 90 days before `to`. |

Status codes are precise: an unknown metric is `404`, a wrong-kind request
(`/metric` on a Snapshot, `/series` on a ReadThrough) is `400`, a malformed
`from`/`to` date is `400`, and an underlying query error is `500`. Routes use Go
1.22+ method+wildcard patterns, so `/metric/foo/bar` does not match and 404s.

```go
mux := http.NewServeMux()
insights := adminhttp.NewHandler(g)
mux.Handle("/admin/insights/", requireAdmin(http.StripPrefix("/admin/insights", insights)))
```

## Experiment analysis

The `experiments` subpackage is a small, **dependency-free** A/B analysis core —
no database and no gnomon-engine imports, just the standard library. It operates
on raw counts, so you can feed it exposure/conversion tallies from any source
(including a gnomon ReadThrough metric).

An **arm** is one variation's observed counts:

```go
type Arm struct {
    Name        string
    N           int // exposed units
    Conversions int // units that converted
}
```

### Frequentist — `Analyze`

`Analyze(control Arm, treatments []Arm, conf float64) []Result` compares each
treatment to control with a **two-proportion z-test** (pooled SE for the test
statistic and p-value; unpooled Wald SE for the confidence interval). The
critical value comes from an Acklam inverse-normal-CDF approximation.

```go
res := experiments.Analyze(
    experiments.Arm{Name: "control", N: 4000, Conversions: 200},
    []experiments.Arm{{Name: "treatment", N: 4000, Conversions: 260}},
    0.95,
)
// res[0].RelLift     → +0.30  (30% relative lift)
// res[0].PValue      → two-sided p
// res[0].CILow/High  → 95% CI on the ABSOLUTE difference
// res[0].Significant → PValue < (1 - conf)
```

Every field is JSON-tagged and NaN/Inf-sanitized (degenerate inputs — e.g. a
zero-N arm — return a safe, non-significant result rather than `NaN`).

### Bayesian — `AnalyzeBayesian`

`AnalyzeBayesian(control Arm, treatments []Arm, conf float64) []BayesResult`
compares each treatment to control via Monte Carlo over **Beta(1,1) (uniform)
posteriors** — 100,000 draws using Marsaglia–Tsang gamma sampling. It is
**deterministic**: a fixed internal seed means identical inputs yield identical
results (and it never touches the global `math/rand`).

```go
res := experiments.AnalyzeBayesian(control, treatments, 0.95)
// res[0].ProbBeatControl → P(rate_treatment > rate_control)
// res[0].ExpectedLoss    → E[max(0, rate_control - rate_treatment)] — risk of shipping the treatment
// res[0].LiftCILow/High  → 95% credible interval on the absolute lift
```

`ProbBeatControl` and `ExpectedLoss` answer the two questions a ship/no-ship
decision actually turns on — *how likely is this better?* and *how much do I
stand to lose if I'm wrong?* — in a way a p-value does not.

The `experiments` package is standalone: nothing in the gnomon engine or
`adminhttp` calls it. Pull exposure/conversion counts however you like (a gnomon
ReadThrough metric is one natural source) and pass them in.

## How it works

1. **Metrics** are registered at startup into an in-memory registry that
   preserves registration order. Registration validates the whole batch before
   committing any of it.
2. **ReadThrough** `Query(name)` looks the metric up and runs its SQL through the
   `DataSource` live — nothing is stored.
3. **Snapshot** `Capture(ctx, day)` walks every Snapshot metric, runs its SQL,
   parses each row's `value` (and optional `dimension`) into samples, and upserts
   them into the `Store` stamped with the day. Failures are per-metric and
   joined, so one broken query doesn't lose the rest of the day's capture.
4. **Snapshot** `Series(name, from, to)` reads that stored history back as
   ordered points for plotting.
5. The **HTTP handler** projects all of the above to JSON with string-valued
   enums, so a frontend can render without knowing Go.

## Testing

```bash
go test ./...                 # core, adminhttp, experiments (sourcepg skips without a DB)
go test -race ./...           # with the race detector

# exercise the Postgres adapter against a real database:
export GNOMON_TEST_DATABASE_URL='postgres://user:pass@localhost:5432/gnomon_test?sslmode=disable'
go test ./sourcepg/...
```

## Roadmap

- **Core engine** ✓ — ReadThrough + Snapshot metrics, daily capture, JSON serving.
- **Experiment analysis** ✓ — frequentist two-proportion z-test and Bayesian
  beta-binomial lift/significance.
- **Future** — additional chart/unit types as hosts need them; convenience
  wiring from a gnomon metric straight into `experiments` arms.

## License

Apache-2.0. See [LICENSE](LICENSE).
