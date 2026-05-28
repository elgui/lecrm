package config

import "testing"

func TestCollectDealProperties(t *testing.T) {
	cfg := &MethodologyConfig{
		PipelineStages: []PipelineStage{
			{
				Key: "stage-a",
				StageProperties: []StageProperty{
					{Key: "contact_name", Type: "string", Required: true},
					{Key: "budget", Type: "number", Required: false},
				},
			},
			{
				Key: "stage-b",
				StageProperties: []StageProperty{
					{Key: "budget", Type: "number", Required: false},
					{Key: "close_date", Type: "date", Required: true},
				},
			},
		},
	}

	props := collectDealProperties(cfg)
	if len(props) != 3 {
		t.Fatalf("got %d properties, want 3 (budget deduped)", len(props))
	}

	keys := make(map[string]bool)
	for _, p := range props {
		keys[p.Key] = true
	}
	for _, want := range []string{"contact_name", "budget", "close_date"} {
		if !keys[want] {
			t.Errorf("missing expected property %q", want)
		}
	}
}

func TestCollectDealPropertiesEmpty(t *testing.T) {
	cfg := &MethodologyConfig{}
	props := collectDealProperties(cfg)
	if len(props) != 0 {
		t.Fatalf("got %d properties for empty config, want 0", len(props))
	}
}

func TestCollectDealPropertiesFirstWins(t *testing.T) {
	cfg := &MethodologyConfig{
		PipelineStages: []PipelineStage{
			{
				Key: "stage-a",
				StageProperties: []StageProperty{
					{Key: "amount", Type: "number", Required: true},
				},
			},
			{
				Key: "stage-b",
				StageProperties: []StageProperty{
					{Key: "amount", Type: "string", Required: false},
				},
			},
		},
	}

	props := collectDealProperties(cfg)
	if len(props) != 1 {
		t.Fatalf("got %d properties, want 1", len(props))
	}
	if props[0].Type != "number" {
		t.Errorf("first occurrence should win: got type %q, want %q", props[0].Type, "number")
	}
}
