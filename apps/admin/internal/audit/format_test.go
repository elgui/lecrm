package audit

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFormatTable_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := FormatTable(&buf, nil); err != nil {
		t.Fatalf("FormatTable: %v", err)
	}
	if !strings.Contains(buf.String(), "no audit entries") {
		t.Errorf("want empty marker, got %q", buf.String())
	}
}

func TestFormatTable_Rows(t *testing.T) {
	ws := uuid.New()
	entries := []Entry{
		{
			ID:          42,
			OccurredAt:  time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC),
			Event:       "config.template.applied",
			WorkspaceID: &ws,
			ActorType:   "human_api",
			Payload:     map[string]any{"template": "gbconsult-default"},
		},
	}
	var buf bytes.Buffer
	if err := FormatTable(&buf, entries); err != nil {
		t.Fatalf("FormatTable: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "config.template.applied") {
		t.Errorf("missing event in output: %q", out)
	}
	if !strings.Contains(out, "human_api") {
		t.Errorf("missing actor in output: %q", out)
	}
	if !strings.Contains(out, "gbconsult-default") {
		t.Errorf("missing payload in output: %q", out)
	}
}

func TestFormatJSON_Roundtrip(t *testing.T) {
	entries := []Entry{
		{ID: 1, Event: "x", Payload: map[string]any{"k": "v"}},
	}
	var buf bytes.Buffer
	if err := FormatJSON(&buf, entries); err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	if !strings.Contains(buf.String(), `"event": "x"`) {
		t.Errorf("missing event in JSON: %q", buf.String())
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct{ in, want string }{
		{"short", "short"},
		{"this-is-too-long-to-fit", "this-is-too-long-to-fit"[:10-3] + "..."},
	}
	for _, c := range cases {
		got := truncate(c.in, 10)
		if got != c.want {
			t.Errorf("truncate(%q,10) = %q want %q", c.in, got, c.want)
		}
	}
}
