package config

import "testing"

func TestToFlatMapAndMergeKeys(t *testing.T) {
	cfg := &MethodologyConfig{
		VersionSeq:   1,
		TemplateName: "test",
		AcquisitionChannels: []AcquisitionChannel{
			{Key: "referral", Label: "Referral", Attribution: map[string]string{"source": "partner"}},
		},
		PipelineStages: []PipelineStage{
			{
				Key:        "discovery",
				Label:      "Discovery",
				OrderIndex: 1,
				StageProperties: []StageProperty{
					{Key: "name", Type: "string", Required: true},
				},
			},
		},
		ColorCoding: map[string]string{"discovery": "#3B82F6"},
	}

	flat, err := toFlatMap(cfg)
	if err != nil {
		t.Fatalf("toFlatMap: %v", err)
	}

	checks := map[string]string{
		"acquisition_channels[0].key":                  "referral",
		"acquisition_channels[0].attribution.source":   "partner",
		"pipeline_stages[0].key":                       "discovery",
		"pipeline_stages[0].stage_properties[0].key":   "name",
		"pipeline_stages[0].stage_properties[0].type":  "string",
		"color_coding.discovery":                       "#3B82F6",
	}
	for path, want := range checks {
		got, ok := flat[path]
		if !ok {
			t.Errorf("path %q missing from flat map", path)
			continue
		}
		if got != want {
			t.Errorf("path %q: got %q, want %q", path, got, want)
		}
	}
}

func TestGroupByTopLevel(t *testing.T) {
	entries := []DiffEntry{
		{Path: "acquisition_channels[0].key", Type: "changed"},
		{Path: "acquisition_channels[1].label", Type: "added"},
		{Path: "pipeline_stages[0].key", Type: "removed"},
		{Path: "color_coding.discovery", Type: "changed"},
	}

	groups := groupByTopLevel(entries)
	if len(groups["acquisition_channels"]) != 2 {
		t.Errorf("acquisition_channels group: got %d entries, want 2", len(groups["acquisition_channels"]))
	}
	if len(groups["pipeline_stages"]) != 1 {
		t.Errorf("pipeline_stages group: got %d entries, want 1", len(groups["pipeline_stages"]))
	}
	if len(groups["color_coding"]) != 1 {
		t.Errorf("color_coding group: got %d entries, want 1", len(groups["color_coding"]))
	}
}
