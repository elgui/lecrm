package reports

import (
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time {
	// 2026-06-15 — Q2, June. Chosen so month/quarter/year windows are distinct.
	return time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
}

func TestNormalizeAndValidate_Defaults(t *testing.T) {
	d := Definition{}
	if err := d.normalizeAndValidate(); err != nil {
		t.Fatalf("empty definition should normalize, got %v", err)
	}
	if d.Metric != MetricDealCount || d.Dimension != DimNone || d.Period != PeriodAll {
		t.Fatalf("defaults wrong: %+v", d)
	}
}

func TestNormalizeAndValidate_Rejections(t *testing.T) {
	cases := []struct {
		name string
		def  Definition
	}{
		{"bad metric", Definition{Metric: "evil"}},
		{"bad period", Definition{Period: "fortnight"}},
		{"bad dimension", Definition{Dimension: "galaxy"}},
		{"bad custom key", Definition{Dimension: "custom:bad key; DROP TABLE deals"}},
		{"yoy without period", Definition{Period: PeriodAll, CompareYoY: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			def := c.def
			if err := def.normalizeAndValidate(); err == nil {
				t.Fatalf("expected validation error for %s", c.name)
			}
		})
	}
}

func TestBuildRunQuery_DimensionsAndMetrics(t *testing.T) {
	now := fixedNow()

	t.Run("count by stage joins pipeline_stages", func(t *testing.T) {
		sql, args, plan, err := BuildRunQuery(Definition{Metric: MetricDealCount, Dimension: DimStage, Period: PeriodAll}, now)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sql, "LEFT JOIN pipeline_stages") {
			t.Errorf("missing stage join: %s", sql)
		}
		if !strings.Contains(sql, "COUNT(*)") {
			t.Errorf("missing count: %s", sql)
		}
		if plan.HasPrior {
			t.Error("PeriodAll must not produce a prior column")
		}
		if len(args) != 0 {
			t.Errorf("PeriodAll/no-custom should bind no args, got %d", len(args))
		}
	})

	t.Run("amount sum by company joins companies", func(t *testing.T) {
		sql, _, _, err := BuildRunQuery(Definition{Metric: MetricDealAmountSum, Dimension: DimCompany, Period: PeriodAll}, now)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sql, "LEFT JOIN companies") {
			t.Errorf("missing company join: %s", sql)
		}
		if !strings.Contains(sql, "SUM(d.amount)") {
			t.Errorf("missing sum: %s", sql)
		}
	})

	t.Run("win rate uses closed_at", func(t *testing.T) {
		sql, _, _, err := BuildRunQuery(Definition{Metric: MetricWinRate, Dimension: DimOwner, Period: PeriodAll}, now)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sql, "closed_at IS NOT NULL") {
			t.Errorf("win rate must reference closed_at: %s", sql)
		}
	})
}

func TestBuildRunQuery_CustomDimensionBindsKeyAsParam(t *testing.T) {
	now := fixedNow()
	sql, args, _, err := BuildRunQuery(Definition{Metric: MetricDealCount, Dimension: "custom:source_du_lead", Period: PeriodAll}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "LEFT JOIN objects o") {
		t.Errorf("custom dim must join objects: %s", sql)
	}
	if !strings.Contains(sql, "o.data->>$1") {
		t.Errorf("custom key must be a bound param ($1), not interpolated: %s", sql)
	}
	if strings.Contains(sql, "source_du_lead") {
		t.Errorf("custom key must NOT appear literally in SQL: %s", sql)
	}
	if len(args) != 1 || args[0] != "source_du_lead" {
		t.Errorf("custom key must be the first bound arg, got %v", args)
	}
}

func TestBuildRunQuery_YoYAddsPriorColumnAndWindows(t *testing.T) {
	now := fixedNow()
	sql, args, plan, err := BuildRunQuery(
		Definition{Metric: MetricDealCount, Dimension: DimStage, Period: PeriodYear, CompareYoY: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasPrior {
		t.Fatal("expected prior column for YoY")
	}
	if !strings.Contains(sql, "AS prior") {
		t.Errorf("missing prior column: %s", sql)
	}
	if plan.CurLabel != "2026" || plan.PriorLabel != "2025" {
		t.Errorf("year labels wrong: cur=%q prior=%q", plan.CurLabel, plan.PriorLabel)
	}
	// Args: cur.start, cur.end, prior.start, prior.end (4 timestamptz).
	if len(args) != 4 {
		t.Fatalf("expected 4 time args, got %d: %v", len(args), args)
	}
	curStart, _ := args[0].(time.Time)
	priorStart, _ := args[2].(time.Time)
	if curStart.Year() != 2026 || curStart.Month() != 1 {
		t.Errorf("current window should start 2026-01, got %v", curStart)
	}
	if priorStart.Year() != 2025 || priorStart.Month() != 1 {
		t.Errorf("prior window should start 2025-01, got %v", priorStart)
	}
}

func TestPeriodWindows_Labels(t *testing.T) {
	now := fixedNow()
	_, _, curM, priorM := periodWindows(PeriodMonth, now)
	if curM != "juin 2026" || priorM != "juin 2025" {
		t.Errorf("month labels: cur=%q prior=%q", curM, priorM)
	}
	_, _, curQ, priorQ := periodWindows(PeriodQuarter, now)
	if curQ != "T2 2026" || priorQ != "T2 2025" {
		t.Errorf("quarter labels: cur=%q prior=%q", curQ, priorQ)
	}
}
