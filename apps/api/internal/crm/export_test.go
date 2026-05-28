package crm

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCSVFilename(t *testing.T) {
	now := time.Date(2026, 5, 25, 14, 3, 0, 0, time.UTC)
	if got := csvFilename("contacts", now); got != "contacts_2026-05-25.csv" {
		t.Errorf("csvFilename = %q, want contacts_2026-05-25.csv", got)
	}
}

func TestCustomPropertyColumns_SortedUnion(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	props := map[uuid.UUID]map[string]any{
		id1: {"region": "EU", "tier": "gold"},
		id2: {"tier": "silver", "vip": true},
	}
	got := customPropertyColumns(props)
	want := []string{"region", "tier", "vip"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("columns = %v, want %v (sorted union)", got, want)
	}
}

func TestCustomPropertyColumns_Empty(t *testing.T) {
	if got := customPropertyColumns(map[uuid.UUID]map[string]any{}); len(got) != 0 {
		t.Errorf("empty props -> %v, want []", got)
	}
}

func TestFormatCSVValue(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{true, "true"},
		{false, "false"},
		{float64(42), "42"},
		{float64(3.5), "3.5"},
		{json.Number("1234567890123"), "1234567890123"},
		{[]any{"a", "b"}, "[a b]"},
	}
	for _, c := range cases {
		if got := formatCSVValue(c.in); got != c.want {
			t.Errorf("formatCSVValue(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCustomCells_AlignsToColumns(t *testing.T) {
	cols := []string{"region", "tier", "vip"}
	// A row missing "vip" must still emit a cell for it (empty), so every
	// CSV record has the same arity as the header.
	got := customCells(map[string]any{"region": "EU", "tier": "gold"}, cols)
	want := []string{"EU", "gold", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("customCells = %v, want %v", got, want)
	}
}

func TestCustomCells_NilProps(t *testing.T) {
	cols := []string{"a", "b"}
	got := customCells(nil, cols)
	if !reflect.DeepEqual(got, []string{"", ""}) {
		t.Errorf("nil props -> %v, want two empty cells", got)
	}
}

func TestPrefixCols(t *testing.T) {
	got := prefixCols([]string{"region", "tier"})
	want := []string{"cf_region", "cf_tier"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("prefixCols = %v, want %v", got, want)
	}
}

func TestScalarHelpers(t *testing.T) {
	s := "x"
	f := 1.5
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if strOrEmpty(nil) != "" || strOrEmpty(&s) != "x" {
		t.Error("strOrEmpty")
	}
	if floatOrEmpty(nil) != "" || floatOrEmpty(&f) != "1.5" {
		t.Error("floatOrEmpty")
	}
	if timeOrEmpty(nil) != "" || timeOrEmpty(&ts) != "2026-01-02T03:04:05Z" {
		t.Error("timeOrEmpty")
	}
}
