package reports

// Native report query engine (ADR-010 / integrator gap-closure).
//
// WHY NATIVE (not Cube): the Cube.dev embed stack (container + per-workspace
// RO roles + /cube/embed frontend + LECRM_CUBE_JWT_SECRET) is not provisioned
// on the public demo, so the iframe path renders an honest "coming soon"
// placeholder there. This engine instead runs aggregation SQL directly against
// the caller's workspace schema — the same search_path-scoped transaction used
// by every other CRM read (capability.ReadTx) — so reporting is live wherever
// the API+DB run, including the demo. Cube remains wired (handler.go embed
// token) for deployments that provision it; the two paths are independent.
//
// SAFETY: every metric / dimension SQL fragment is selected from a fixed
// allow-list (the switch statements below). No user-supplied string is ever
// concatenated into SQL. The one free-form input — a custom-property key for
// the `custom:<key>` dimension — is passed as a bound query parameter to the
// jsonb `->>` operator (never interpolated), and additionally validated against
// a conservative identifier pattern. Time-window bounds are bound parameters.

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Metric identifiers (allow-listed).
const (
	MetricDealCount     = "deal_count"
	MetricDealAmountSum = "deal_amount_sum"
	MetricWinRate       = "win_rate"
)

// Dimension kinds. The wire `dimension` field is either one of the bare kinds
// or "custom:<property_key>".
const (
	DimNone    = "none"
	DimStage   = "stage"
	DimOwner   = "owner"
	DimCompany = "company"
	dimCustom  = "custom" // prefix form: "custom:<key>"
)

// Period identifiers (allow-listed).
const (
	PeriodAll     = "all"
	PeriodMonth   = "month"
	PeriodQuarter = "quarter"
	PeriodYear    = "year"
)

// customKeyRe constrains custom-property keys to the same shape the metadata
// engine allows (alphanumerics + underscore). Defence-in-depth only: the key
// is bound as a parameter regardless, never interpolated.
var customKeyRe = regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`)

// Definition is a saved/ad-hoc report definition. It is also the JSONB payload
// persisted in the workspace `objects` table (object_type='saved_report').
type Definition struct {
	Name       string `json:"name"`
	Metric     string `json:"metric"`
	Dimension  string `json:"dimension"`
	Period     string `json:"period"`
	CompareYoY bool   `json:"compare_yoy"`
}

// ValidationError is returned for a malformed definition (maps to HTTP 400).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// dimensionKind splits a wire dimension into its kind and (for custom) key.
func dimensionKind(dimension string) (kind, key string) {
	if strings.HasPrefix(dimension, dimCustom+":") {
		return dimCustom, strings.TrimPrefix(dimension, dimCustom+":")
	}
	return dimension, ""
}

// Validate checks a definition against the allow-lists. Empty metric/dimension/
// period default to count / none / all so a half-filled builder still runs.
func (d *Definition) normalizeAndValidate() error {
	if d.Metric == "" {
		d.Metric = MetricDealCount
	}
	if d.Dimension == "" {
		d.Dimension = DimNone
	}
	if d.Period == "" {
		d.Period = PeriodAll
	}
	switch d.Metric {
	case MetricDealCount, MetricDealAmountSum, MetricWinRate:
	default:
		return &ValidationError{Msg: "unknown metric: " + d.Metric}
	}
	switch d.Period {
	case PeriodAll, PeriodMonth, PeriodQuarter, PeriodYear:
	default:
		return &ValidationError{Msg: "unknown period: " + d.Period}
	}
	kind, key := dimensionKind(d.Dimension)
	switch kind {
	case DimNone, DimStage, DimOwner, DimCompany:
	case dimCustom:
		if !customKeyRe.MatchString(key) {
			return &ValidationError{Msg: "invalid custom-property key in dimension"}
		}
	default:
		return &ValidationError{Msg: "unknown dimension: " + d.Dimension}
	}
	// N-1 comparison only makes sense against a bounded period.
	if d.CompareYoY && d.Period == PeriodAll {
		return &ValidationError{Msg: "compare_yoy requires a period (month/quarter/year)"}
	}
	return nil
}

// window is a half-open [start, end) time range.
type window struct {
	start time.Time
	end   time.Time
}

// periodWindows returns the current window and (when applicable) the same
// window one year earlier, plus French labels for each. For PeriodAll the
// current window covers all time (zero value sentinels) and prior is nil.
func periodWindows(period string, now time.Time) (cur window, prior *window, curLabel, priorLabel string) {
	y, m, _ := now.Date()
	loc := now.Location()
	switch period {
	case PeriodMonth:
		start := time.Date(y, m, 1, 0, 0, 0, 0, loc)
		cur = window{start, start.AddDate(0, 1, 0)}
		p := window{start.AddDate(-1, 0, 0), cur.end.AddDate(-1, 0, 0)}
		prior = &p
		curLabel = frMonth(start)
		priorLabel = frMonth(p.start)
	case PeriodQuarter:
		q := (int(m) - 1) / 3 // 0..3
		startMonth := time.Month(q*3 + 1)
		start := time.Date(y, startMonth, 1, 0, 0, 0, 0, loc)
		cur = window{start, start.AddDate(0, 3, 0)}
		p := window{start.AddDate(-1, 0, 0), cur.end.AddDate(-1, 0, 0)}
		prior = &p
		curLabel = "T" + strconv.Itoa(q+1) + " " + strconv.Itoa(y)
		priorLabel = "T" + strconv.Itoa(q+1) + " " + strconv.Itoa(y-1)
	case PeriodYear:
		start := time.Date(y, 1, 1, 0, 0, 0, 0, loc)
		cur = window{start, start.AddDate(1, 0, 0)}
		p := window{start.AddDate(-1, 0, 0), start}
		prior = &p
		curLabel = strconv.Itoa(y)
		priorLabel = strconv.Itoa(y - 1)
	default: // PeriodAll
		curLabel = "Tout l'historique"
	}
	return cur, prior, curLabel, priorLabel
}

var frMonths = [...]string{
	"janvier", "février", "mars", "avril", "mai", "juin",
	"juillet", "août", "septembre", "octobre", "novembre", "décembre",
}

func frMonth(t time.Time) string {
	return frMonths[int(t.Month())-1] + " " + strconv.Itoa(t.Year())
}

// argBuilder accumulates positional query parameters and hands back $N
// placeholders. Casts are appended explicitly because the pool runs in
// simple-protocol mode (see internal/db), which does not infer parameter types.
type argBuilder struct {
	args []any
}

func (a *argBuilder) add(v any, cast string) string {
	a.args = append(a.args, v)
	p := "$" + strconv.Itoa(len(a.args))
	if cast != "" {
		p += "::" + cast
	}
	return p
}

// metricExpr returns the SQL aggregate for a metric, optionally constrained to
// a [start,end) window via FILTER. When ph is empty the aggregate is unfiltered
// (whole table / PeriodAll).
func metricExpr(metric string, startPH, endPH string) string {
	var filter string
	if startPH != "" {
		filter = " FILTER (WHERE d.created_at >= " + startPH + " AND d.created_at < " + endPH + ")"
	}
	switch metric {
	case MetricDealAmountSum:
		return "COALESCE(SUM(d.amount)" + filter + ", 0)::float8"
	case MetricWinRate:
		// Share of deals that have been closed (closed_at set). The v0 schema
		// has no first-class won/lost flag — the terminal pipeline stage is the
		// combined "Gagné / Perdu" — so closure is the agreed win proxy until a
		// dedicated outcome field lands. Documented here so the metric is not
		// silently mistaken for true won/total.
		var closedFilter, totalFilter string
		if startPH != "" {
			closedFilter = " FILTER (WHERE d.closed_at IS NOT NULL AND d.created_at >= " + startPH + " AND d.created_at < " + endPH + ")"
			totalFilter = filter
		} else {
			closedFilter = " FILTER (WHERE d.closed_at IS NOT NULL)"
		}
		return "COALESCE(COUNT(*)" + closedFilter + "::float8 / NULLIF(COUNT(*)" + totalFilter + ", 0), 0)::float8"
	default: // MetricDealCount
		return "COUNT(*)" + filter + "::float8"
	}
}

// runPlan describes the shape of the rows BuildRunQuery's SQL returns.
type runPlan struct {
	HasPrior   bool
	CurLabel   string
	PriorLabel string
}

// BuildRunQuery turns a validated Definition into a single aggregation query
// plus its bound arguments. The query returns columns: dim_label (text),
// current (float8), and — when comparing — prior (float8), ordered by an
// internal dim_order then current value. Pure and side-effect free so the SQL
// shape can be asserted without a database.
func BuildRunQuery(def Definition, now time.Time) (sql string, args []any, plan runPlan, err error) {
	if err := def.normalizeAndValidate(); err != nil {
		return "", nil, runPlan{}, err
	}
	cur, prior, curLabel, priorLabel := periodWindows(def.Period, now)
	ab := &argBuilder{}

	// Dimension: label expression + optional join + group-by key.
	var dimLabel, dimOrder, join, groupBy string
	kind, key := dimensionKind(def.Dimension)
	switch kind {
	case DimStage:
		join = " LEFT JOIN pipeline_stages ps ON ps.id = d.stage_id"
		dimLabel = "COALESCE(ps.name, 'Sans étape')"
		dimOrder = "COALESCE(ps.order_index, 2147483647)"
		groupBy = "ps.name, ps.order_index"
	case DimOwner:
		dimLabel = "COALESCE(d.owner_id::text, 'Non attribué')"
		dimOrder = "0"
		groupBy = "d.owner_id"
	case DimCompany:
		join = " LEFT JOIN companies c ON c.id = d.company_id"
		dimLabel = "COALESCE(c.name, 'Sans société')"
		dimOrder = "0"
		groupBy = "c.name"
	case dimCustom:
		keyPH := ab.add(key, "text")
		join = " LEFT JOIN objects o ON o.parent_type = 'deal' AND o.parent_id = d.id AND o.object_type = 'custom_properties'"
		dimLabel = "COALESCE(o.data->>" + keyPH + ", '—')"
		dimOrder = "0"
		groupBy = "o.data->>" + keyPH
	default: // DimNone
		dimLabel = "'Total'"
		dimOrder = "0"
		groupBy = ""
	}

	// Metric columns + the time predicate that bounds the scan.
	var curExpr, priorExpr, whereClause string
	plan.CurLabel = curLabel
	if def.Period == PeriodAll {
		curExpr = metricExpr(def.Metric, "", "")
	} else {
		cs := ab.add(cur.start, "timestamptz")
		ce := ab.add(cur.end, "timestamptz")
		curExpr = metricExpr(def.Metric, cs, ce)
		if def.CompareYoY && prior != nil {
			ps := ab.add(prior.start, "timestamptz")
			pe := ab.add(prior.end, "timestamptz")
			priorExpr = metricExpr(def.Metric, ps, pe)
			// Bound the scan to [prior.start, cur.end): both windows live inside
			// it (the prior window is a year before the current one).
			whereClause = " WHERE d.created_at >= " + ps + " AND d.created_at < " + ce
			plan.HasPrior = true
			plan.PriorLabel = priorLabel
		} else {
			whereClause = " WHERE d.created_at >= " + cs + " AND d.created_at < " + ce
		}
	}

	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(dimLabel)
	b.WriteString(" AS dim_label, ")
	b.WriteString(curExpr)
	b.WriteString(" AS current")
	if plan.HasPrior {
		b.WriteString(", ")
		b.WriteString(priorExpr)
		b.WriteString(" AS prior")
	}
	b.WriteString(", ")
	b.WriteString(dimOrder)
	b.WriteString(" AS dim_order FROM deals d")
	b.WriteString(join)
	b.WriteString(whereClause)
	if groupBy != "" {
		b.WriteString(" GROUP BY ")
		b.WriteString(groupBy)
	}
	b.WriteString(" ORDER BY dim_order ASC, current DESC, dim_label ASC")
	b.WriteString(" LIMIT 200")

	return b.String(), ab.args, plan, nil
}

// describe is a small helper that returns a human title for a metric (FR),
// used by the handler to enrich the run response.
func metricLabel(metric string) string {
	switch metric {
	case MetricDealAmountSum:
		return "Montant total (€)"
	case MetricWinRate:
		return "Taux de réussite"
	default:
		return "Nombre d'affaires"
	}
}
