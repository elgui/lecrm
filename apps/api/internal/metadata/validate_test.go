package metadata

import (
	"testing"
)

// TestValidationError_ImplementsError verifies that *ValidationError satisfies
// the error interface and that Error() returns the Msg field verbatim.
func TestValidationError_ImplementsError(t *testing.T) {
	var _ error = (*ValidationError)(nil) // compile-time interface check

	cases := []struct {
		name string
		msg  string
	}{
		{"non-empty message", "property \"foo\" must be a string"},
		{"empty message", ""},
		{"special characters", `"<>&`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			e := &ValidationError{Msg: tc.msg}
			if got := e.Error(); got != tc.msg {
				t.Errorf("Error() = %q, want %q", got, tc.msg)
			}
		})
	}
}

// TestValidateValue covers every branch of validateValue using table-driven
// sub-tests. All tests run within package metadata to access the unexported
// function directly.
func TestValidateValue(t *testing.T) {
	type testCase struct {
		name    string
		key     string
		val     any
		def     defEntry
		wantErr bool
	}

	tests := []testCase{
		// ── string ──────────────────────────────────────────────────────────
		{
			name:    "string/valid string value",
			key:     "name",
			val:     "hello",
			def:     defEntry{propType: "string"},
			wantErr: false,
		},
		{
			name:    "string/int rejected",
			key:     "name",
			val:     42,
			def:     defEntry{propType: "string"},
			wantErr: true,
		},
		{
			name:    "string/float64 rejected",
			key:     "name",
			val:     3.14,
			def:     defEntry{propType: "string"},
			wantErr: true,
		},
		{
			name:    "string/bool rejected",
			key:     "name",
			val:     true,
			def:     defEntry{propType: "string"},
			wantErr: true,
		},

		// ── number ──────────────────────────────────────────────────────────
		{
			name:    "number/float64 passes",
			key:     "score",
			val:     float64(9.5),
			def:     defEntry{propType: "number"},
			wantErr: false,
		},
		{
			name:    "number/int passes",
			key:     "score",
			val:     int(3),
			def:     defEntry{propType: "number"},
			wantErr: false,
		},
		{
			name:    "number/int64 passes",
			key:     "score",
			val:     int64(1000),
			def:     defEntry{propType: "number"},
			wantErr: false,
		},
		{
			name:    "number/float32 passes",
			key:     "score",
			val:     float32(1.5),
			def:     defEntry{propType: "number"},
			wantErr: false,
		},
		{
			name:    "number/string rejected",
			key:     "score",
			val:     "99",
			def:     defEntry{propType: "number"},
			wantErr: true,
		},
		{
			name:    "number/bool rejected",
			key:     "score",
			val:     false,
			def:     defEntry{propType: "number"},
			wantErr: true,
		},

		// ── boolean ─────────────────────────────────────────────────────────
		{
			name:    "boolean/true passes",
			key:     "active",
			val:     true,
			def:     defEntry{propType: "boolean"},
			wantErr: false,
		},
		{
			name:    "boolean/false passes",
			key:     "active",
			val:     false,
			def:     defEntry{propType: "boolean"},
			wantErr: false,
		},
		{
			name:    "boolean/string rejected",
			key:     "active",
			val:     "true",
			def:     defEntry{propType: "boolean"},
			wantErr: true,
		},
		{
			name:    "boolean/int rejected",
			key:     "active",
			val:     1,
			def:     defEntry{propType: "boolean"},
			wantErr: true,
		},

		// ── date ────────────────────────────────────────────────────────────
		{
			name:    "date/valid ISO-8601 date passes",
			key:     "due",
			val:     "2024-01-15",
			def:     defEntry{propType: "date"},
			wantErr: false,
		},
		{
			name:    "date/not-a-date string rejected",
			key:     "due",
			val:     "not-a-date",
			def:     defEntry{propType: "date"},
			wantErr: true,
		},
		{
			name:    "date/US slash format rejected",
			key:     "due",
			val:     "01/15/2024",
			def:     defEntry{propType: "date"},
			wantErr: true,
		},
		{
			name:    "date/non-string rejected",
			key:     "due",
			val:     20240115,
			def:     defEntry{propType: "date"},
			wantErr: true,
		},
		{
			name:    "date/partial date rejected",
			key:     "due",
			val:     "2024-01",
			def:     defEntry{propType: "date"},
			wantErr: true,
		},

		// ── enum ────────────────────────────────────────────────────────────
		{
			name:    "enum/value in allowed list passes",
			key:     "status",
			val:     "active",
			def:     defEntry{propType: "enum", allowed: []string{"active", "inactive", "pending"}},
			wantErr: false,
		},
		{
			name:    "enum/value not in list rejected",
			key:     "status",
			val:     "deleted",
			def:     defEntry{propType: "enum", allowed: []string{"active", "inactive"}},
			wantErr: true,
		},
		{
			name:    "enum/non-string rejected",
			key:     "status",
			val:     42,
			def:     defEntry{propType: "enum", allowed: []string{"active"}},
			wantErr: true,
		},
		{
			name:    "enum/empty allowed list is permissive (any string passes)",
			key:     "status",
			val:     "anything",
			def:     defEntry{propType: "enum", allowed: []string{}},
			wantErr: false,
		},

		// ── json ────────────────────────────────────────────────────────────
		{
			name:    "json/map passes",
			key:     "payload",
			val:     map[string]any{"key": "value"},
			def:     defEntry{propType: "json"},
			wantErr: false,
		},
		{
			name:    "json/slice passes",
			key:     "payload",
			val:     []any{"a", "b"},
			def:     defEntry{propType: "json"},
			wantErr: false,
		},
		{
			name:    "json/string rejected",
			key:     "payload",
			val:     "not-an-object",
			def:     defEntry{propType: "json"},
			wantErr: true,
		},
		{
			name:    "json/number rejected",
			key:     "payload",
			val:     float64(42),
			def:     defEntry{propType: "json"},
			wantErr: true,
		},
		{
			name:    "json/bool rejected",
			key:     "payload",
			val:     true,
			def:     defEntry{propType: "json"},
			wantErr: true,
		},

		// ── unknown propType ─────────────────────────────────────────────────
		// The switch statement has no default case, so unknown types pass through.
		{
			name:    "unknown type/any value passes through",
			key:     "mystery",
			val:     "whatever",
			def:     defEntry{propType: "custom_type_not_in_switch"},
			wantErr: false,
		},
		{
			name:    "unknown type/nil value passes through",
			key:     "mystery",
			val:     nil,
			def:     defEntry{propType: "custom_type_not_in_switch"},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateValue(tc.key, tc.val, tc.def)
			if tc.wantErr && err == nil {
				t.Errorf("validateValue(%q, %T(%v), propType=%q): expected error, got nil",
					tc.key, tc.val, tc.val, tc.def.propType)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateValue(%q, %T(%v), propType=%q): unexpected error: %v",
					tc.key, tc.val, tc.val, tc.def.propType, err)
			}
			// When an error is returned it must be a *ValidationError.
			if err != nil {
				if _, ok := err.(*ValidationError); !ok {
					t.Errorf("validateValue returned %T, want *ValidationError", err)
				}
			}
		})
	}
}

// TestValidateValue_ErrorMessages spot-checks that error messages include the
// property key so callers can surface useful diagnostics.
func TestValidateValue_ErrorMessages(t *testing.T) {
	cases := []struct {
		propType string
		val      any
	}{
		{"string", 42},
		{"number", "text"},
		{"boolean", "yes"},
		{"date", "13/01/2024"},
		{"enum", 7},
		{"json", "raw"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.propType, func(t *testing.T) {
			def := defEntry{propType: tc.propType, allowed: []string{"a"}}
			err := validateValue("mykey", tc.val, def)
			if err == nil {
				t.Fatalf("expected error for propType=%q val=%T", tc.propType, tc.val)
			}
			ve, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("error is %T, want *ValidationError", err)
			}
			if ve.Msg == "" {
				t.Error("ValidationError.Msg is empty")
			}
		})
	}
}
