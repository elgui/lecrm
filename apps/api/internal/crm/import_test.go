package crm

// Unit tests for the pure CSV-import logic (parsing, column-mapping
// heuristics, type coercion, per-row validation). These run WITHOUT the
// `integration` build tag — i.e. with no Docker/Postgres — so they execute in
// the plain `go test ./...` gate. They cover the exact functions that the
// PR #9 run report found were only ever exercised by Docker integration tests
// (which the per-step verifier never ran), which is how the CSV-import 404
// false-completion slipped through.

import (
	"testing"

	"github.com/google/uuid"
)

func TestSpecForParam(t *testing.T) {
	cases := []struct {
		param  string
		ok     bool
		entity string
	}{
		{"contacts", true, "contact"},
		{"companies", true, "company"},
		{"deals", true, "deal"},
		{"contact", false, ""}, // singular is not the URL segment
		{"", false, ""},
		{"widgets", false, ""},
	}
	for _, c := range cases {
		spec, ok := specForParam(c.param)
		if ok != c.ok {
			t.Errorf("specForParam(%q) ok=%v, want %v", c.param, ok, c.ok)
		}
		if ok && spec.entity != c.entity {
			t.Errorf("specForParam(%q) entity=%q, want %q", c.param, spec.entity, c.entity)
		}
	}
}

func TestParseCSVText(t *testing.T) {
	t.Run("normal with header trim and ragged rows", func(t *testing.T) {
		// Second data row is short (1 field) — FieldsPerRecord=-1 must allow it.
		header, rows, err := parseCSVText(" Prénom , Nom ,E-mail\nAlice,Martin,alice@x.fr\nBob")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"Prénom", "Nom", "E-mail"}
		if len(header) != 3 || header[0] != want[0] || header[1] != want[1] || header[2] != want[2] {
			t.Errorf("header=%v, want %v (trimmed)", header, want)
		}
		if len(rows) != 2 {
			t.Fatalf("got %d data rows, want 2", len(rows))
		}
		if rows[0][0] != "Alice" || rows[1][0] != "Bob" {
			t.Errorf("rows=%v, unexpected content", rows)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		if _, _, err := parseCSVText("   "); err == nil {
			t.Error("expected error for blank CSV, got nil")
		}
	})

	t.Run("malformed quoting", func(t *testing.T) {
		if _, _, err := parseCSVText("a,b\n\"unterminated,c"); err == nil {
			t.Error("expected parse error for bad quoting, got nil")
		}
	})
}

func TestIndexColumns_FirstWinsOnDuplicate(t *testing.T) {
	idx := indexColumns([]string{"email", "name", "email"})
	if idx["email"] != 0 {
		t.Errorf("duplicate header should keep first index: got %d, want 0", idx["email"])
	}
	if idx["name"] != 1 {
		t.Errorf("name index=%d, want 1", idx["name"])
	}
}

func TestValidateMapping(t *testing.T) {
	contact := contactSpec() // has a custom-property parentType
	company := companySpec()  // parentType == "" → no custom props

	t.Run("core fields and empty targets accepted", func(t *testing.T) {
		err := validateMapping(contact, map[string]string{
			"A": "first_name",
			"B": "last_name",
			"C": "", // unmapped column ignored
		})
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("custom property accepted when entity has one", func(t *testing.T) {
		if err := validateMapping(contact, map[string]string{"A": customPropPrefix + "lead_source"}); err != nil {
			t.Errorf("expected nil for cf_ on contact, got %v", err)
		}
	})

	t.Run("custom property rejected when entity has none", func(t *testing.T) {
		if err := validateMapping(company, map[string]string{"A": customPropPrefix + "anything"}); err == nil {
			t.Error("expected error for cf_ on company (no parentType), got nil")
		}
	})

	t.Run("unknown core field rejected", func(t *testing.T) {
		if err := validateMapping(contact, map[string]string{"A": "not_a_field"}); err == nil {
			t.Error("expected error for unknown target, got nil")
		}
	})
}

func TestSuggestMapping(t *testing.T) {
	defs := []importDef{{Key: "lead_source", Label: "Source"}}
	header := []string{"First Name", "NOM", "courriel", "Source", "garbage column"}
	got := suggestMapping(header, contactSpec(), defs)

	want := map[string]string{
		"First Name": "first_name",                   // normalized key match
		"NOM":        "last_name",                     // label "Nom" normalized match
		"courriel":   "email",                         // alias
		"Source":     customPropPrefix + "lead_source", // custom property by label
	}
	for col, target := range want {
		if got[col] != target {
			t.Errorf("suggestMapping[%q]=%q, want %q", col, got[col], target)
		}
	}
	if _, mapped := got["garbage column"]; mapped {
		t.Errorf("unmappable column should be left out, got %q", got["garbage column"])
	}
}

func TestNormalizeKey(t *testing.T) {
	cases := map[string]string{
		"First Name":   "first_name",
		"first-name":   "first_name",
		"  E-mail  ":   "e_mail",
		"Téléphone":    "t_l_phone", // accents are non-[a-z0-9] → separators
		"a__b":         "a_b",
		"!!!":          "",
		"already_norm": "already_norm",
	}
	for in, want := range cases {
		if got := normalizeKey(in); got != want {
			t.Errorf("normalizeKey(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestCoerceCustomValue(t *testing.T) {
	num := importDef{Key: "score", PropertyType: "number"}
	boolean := importDef{Key: "active", PropertyType: "boolean"}
	date := importDef{Key: "signed", PropertyType: "date"}
	enum := importDef{Key: "tier", PropertyType: "enum", Allowed: []string{"gold", "silver"}}
	js := importDef{Key: "meta", PropertyType: "json"}
	str := importDef{Key: "note", PropertyType: "string"}

	t.Run("number", func(t *testing.T) {
		v, err := coerceCustomValue(num, "42.5")
		if f, ok := v.(float64); err != nil || !ok || f != 42.5 {
			t.Errorf("number: v=%v err=%v", v, err)
		}
		if _, err := coerceCustomValue(num, "NaNumber"); err == nil {
			t.Error("expected error for non-number")
		}
	})

	t.Run("boolean incl french tokens", func(t *testing.T) {
		for _, tok := range []string{"true", "1", "oui", "vrai", "Y"} {
			v, err := coerceCustomValue(boolean, tok)
			if b, ok := v.(bool); err != nil || !ok || b != true {
				t.Errorf("bool %q: v=%v err=%v", tok, v, err)
			}
		}
		for _, tok := range []string{"false", "0", "non", "faux"} {
			v, err := coerceCustomValue(boolean, tok)
			if b, ok := v.(bool); err != nil || !ok || b != false {
				t.Errorf("bool %q: v=%v err=%v", tok, v, err)
			}
		}
		if _, err := coerceCustomValue(boolean, "maybe"); err == nil {
			t.Error("expected error for non-boolean token (no Python bool(string) trap)")
		}
	})

	t.Run("date", func(t *testing.T) {
		if _, err := coerceCustomValue(date, "2026-06-01"); err != nil {
			t.Errorf("valid date rejected: %v", err)
		}
		if _, err := coerceCustomValue(date, "01/06/2026"); err == nil {
			t.Error("expected error for non-ISO date")
		}
	})

	t.Run("enum", func(t *testing.T) {
		if _, err := coerceCustomValue(enum, "gold"); err != nil {
			t.Errorf("allowed enum rejected: %v", err)
		}
		if _, err := coerceCustomValue(enum, "bronze"); err == nil {
			t.Error("expected error for disallowed enum value")
		}
	})

	t.Run("json and string", func(t *testing.T) {
		if _, err := coerceCustomValue(js, `{"k":1}`); err != nil {
			t.Errorf("valid json rejected: %v", err)
		}
		if _, err := coerceCustomValue(js, `{bad`); err == nil {
			t.Error("expected error for invalid json")
		}
		v, err := coerceCustomValue(str, "  passes through  ")
		if s, ok := v.(string); err != nil || !ok || s != "  passes through  " {
			t.Errorf("string passthrough: v=%v err=%v", v, err)
		}
	})
}

func TestPrettifyKey(t *testing.T) {
	cases := map[string]string{
		"lead_source": "Lead source",
		"x":           "X",
		"":            "",
	}
	for in, want := range cases {
		if got := prettifyKey(in); got != want {
			t.Errorf("prettifyKey(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestParseAmount(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"1234.56", 1234.56, true},
		{"1 234,56", 1234.56, true},   // French: space thousands + comma decimal
		{"1,234.56", 1234.56, true},   // English: comma thousands + dot decimal
		{"42", 42, true},
		{"", 0, false},
		{"abc", 0, false},
	}
	for _, c := range cases {
		got, err := parseAmount(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("parseAmount(%q)=%v,%v want %v,nil", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("parseAmount(%q) expected error, got %v", c.in, got)
		}
	}
}

func TestSmallValidators(t *testing.T) {
	if validateEmail("alice@example.com") != nil {
		t.Error("valid email rejected")
	}
	if validateEmail("not-an-email") == nil {
		t.Error("invalid email accepted")
	}
	if !validCompanySize("11-50") || validCompanySize("huge") {
		t.Error("validCompanySize bucket check wrong")
	}
	if !validDate("2026-06-01") || validDate("2026-13-01") || validDate("2026-6-1") {
		t.Error("validDate check wrong")
	}
}

// --- importEngine method tests (constructed in-memory, no DB) ---

func newTestEngine(spec importEntitySpec, header []string, mapping map[string]string) *importEngine {
	return &importEngine{
		spec:     spec,
		mapping:  mapping,
		header:   header,
		colIndex: indexColumns(header),
	}
}

func TestImportEngine_CoreValues(t *testing.T) {
	header := []string{"Prénom", "Nom", "E-mail"}
	e := newTestEngine(contactSpec(), header, map[string]string{
		"Prénom": "first_name",
		"Nom":    "last_name",
		"E-mail": "email",
	})
	core := e.coreValues([]string{"  Alice ", "Martin", "alice@x.fr"})
	if core["first_name"] != "Alice" || core["last_name"] != "Martin" || core["email"] != "alice@x.fr" {
		t.Errorf("coreValues trimmed extraction wrong: %v", core)
	}

	// A short record must not panic and just omits the missing column.
	short := newTestEngine(contactSpec(), header, map[string]string{"E-mail": "email"})
	got := short.coreValues([]string{"only-one-col"})
	if _, present := got["email"]; present {
		t.Errorf("out-of-range column should be skipped, got %v", got)
	}
}

func TestImportEngine_CustomValues(t *testing.T) {
	header := []string{"Tier", "Score"}
	e := newTestEngine(contactSpec(), header, map[string]string{
		"Tier":  customPropPrefix + "tier",
		"Score": customPropPrefix + "score",
	})
	defs := map[string]importDef{
		"tier":  {Key: "tier", PropertyType: "enum", Allowed: []string{"gold"}},
		"score": {Key: "score", PropertyType: "number"},
	}
	out, reason := e.customValues([]string{"gold", "10"}, defs)
	if reason != "" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	score, ok := out["score"].(float64)
	if out["tier"] != "gold" || !ok || score != 10 {
		t.Errorf("customValues coercion wrong: %v", out)
	}

	// Bad coercion surfaces a non-empty reason.
	if _, reason := e.customValues([]string{"bronze", "10"}, defs); reason == "" {
		t.Error("expected reason for disallowed enum value")
	}

	// Empty cell leaves the property unset.
	if out, _ := e.customValues([]string{"", "10"}, defs); out["tier"] != nil {
		t.Errorf("empty cell should leave property unset, got %v", out["tier"])
	}
}

func TestImportEngine_RowLabel(t *testing.T) {
	c := newTestEngine(contactSpec(), nil, nil)
	if got := c.rowLabel(map[string]string{"first_name": "Alice", "last_name": "Martin"}); got != "Alice Martin" {
		t.Errorf("contact label=%q, want %q", got, "Alice Martin")
	}
	if got := c.rowLabel(map[string]string{"email": "a@x.fr"}); got != "a@x.fr" {
		t.Errorf("contact label fallback to email failed: %q", got)
	}
	comp := newTestEngine(companySpec(), nil, nil)
	if got := comp.rowLabel(map[string]string{"name": "Acme"}); got != "Acme" {
		t.Errorf("company label=%q, want Acme", got)
	}
}

func TestImportEngine_DedupeKey(t *testing.T) {
	contact := newTestEngine(contactSpec(), nil, nil)
	if got := contact.dedupeKey(map[string]string{"email": "Alice@X.fr"}); got != "email:alice@x.fr" {
		t.Errorf("contact dedupeKey=%q (should lowercase)", got)
	}
	if got := contact.dedupeKey(map[string]string{}); got != "" {
		t.Errorf("no email → empty dedupe key, got %q", got)
	}
	company := newTestEngine(companySpec(), nil, nil)
	if got := company.dedupeKey(map[string]string{"domain": "Acme.COM"}); got != "domain:acme.com" {
		t.Errorf("company prefers domain: %q", got)
	}
	if got := company.dedupeKey(map[string]string{"name": "Acme"}); got != "name:acme" {
		t.Errorf("company falls back to name: %q", got)
	}
	deal := newTestEngine(dealSpec(), nil, nil)
	if got := deal.dedupeKey(map[string]string{"title": "Big deal"}); got != "" {
		t.Errorf("deals never dedupe, got %q", got)
	}
}

func TestImportEngine_ValidateCore(t *testing.T) {
	contact := newTestEngine(contactSpec(), nil, nil)
	if reason := contact.validateCore(map[string]string{"first_name": "Alice", "last_name": "M"}); reason != "" {
		t.Errorf("valid contact rejected: %s", reason)
	}
	if reason := contact.validateCore(map[string]string{"last_name": "M"}); reason == "" {
		t.Error("missing required first_name should be rejected")
	}
	if reason := contact.validateCore(map[string]string{"first_name": "A", "last_name": "B", "email": "bad"}); reason == "" {
		t.Error("invalid email should be rejected")
	}

	deal := newTestEngine(dealSpec(), nil, nil)
	deal.stages = map[string]uuid.UUID{"découverte": uuid.New()}
	if reason := deal.validateCore(map[string]string{"title": "T", "stage": "Découverte"}); reason != "" {
		t.Errorf("known stage (case-insensitive) rejected: %s", reason)
	}
	if reason := deal.validateCore(map[string]string{"title": "T", "stage": "Nope"}); reason == "" {
		t.Error("unknown stage should be rejected")
	}
	if reason := deal.validateCore(map[string]string{"title": "T", "currency": "EURO"}); reason == "" {
		t.Error("non-ISO currency should be rejected")
	}
}
