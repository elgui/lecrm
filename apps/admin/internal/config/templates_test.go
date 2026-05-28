package config

import (
	"encoding/json"
	"os"
	"testing"
)

func TestGBConsultDefaultTemplate(t *testing.T) {
	cfg := GBConsultDefault
	if cfg.TemplateName != "gbconsult-default" {
		t.Errorf("template_name: got %q, want %q", cfg.TemplateName, "gbconsult-default")
	}
	if len(cfg.AcquisitionChannels) != 5 {
		t.Errorf("acquisition_channels: got %d, want 5", len(cfg.AcquisitionChannels))
	}
	if len(cfg.PipelineStages) != 5 {
		t.Errorf("pipeline_stages: got %d, want 5", len(cfg.PipelineStages))
	}
	if len(cfg.Automations) != 3 {
		t.Errorf("automations: got %d, want 3", len(cfg.Automations))
	}
	if len(cfg.ColorCoding) != 5 {
		t.Errorf("color_coding: got %d entries, want 5", len(cfg.ColorCoding))
	}

	for i, stage := range cfg.PipelineStages {
		if stage.OrderIndex != i+1 {
			t.Errorf("stage %q: order_index=%d, want %d", stage.Key, stage.OrderIndex, i+1)
		}
		if len(stage.StageProperties) == 0 {
			t.Errorf("stage %q has no stage_properties", stage.Key)
		}
	}

	for _, stage := range cfg.PipelineStages {
		if _, ok := cfg.ColorCoding[stage.Key]; !ok {
			t.Errorf("stage %q has no color_coding entry", stage.Key)
		}
	}
}

func TestGBConsultDefaultMatchesJSON(t *testing.T) {
	jsonPath := "../../../../docs/templates/gbconsult-default-methodology.json"
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("reference JSON not found at %s: %v", jsonPath, err)
	}

	var fromJSON MethodologyConfig
	if err := json.Unmarshal(raw, &fromJSON); err != nil {
		t.Fatalf("unmarshal reference JSON: %v", err)
	}

	goBytes, err := json.Marshal(GBConsultDefault)
	if err != nil {
		t.Fatalf("marshal Go template: %v", err)
	}
	var fromGo MethodologyConfig
	if err := json.Unmarshal(goBytes, &fromGo); err != nil {
		t.Fatalf("round-trip Go template: %v", err)
	}

	if fromJSON.TemplateName != fromGo.TemplateName {
		t.Errorf("template_name mismatch: JSON=%q Go=%q", fromJSON.TemplateName, fromGo.TemplateName)
	}
	if len(fromJSON.AcquisitionChannels) != len(fromGo.AcquisitionChannels) {
		t.Errorf("acquisition_channels count: JSON=%d Go=%d", len(fromJSON.AcquisitionChannels), len(fromGo.AcquisitionChannels))
	}
	if len(fromJSON.PipelineStages) != len(fromGo.PipelineStages) {
		t.Errorf("pipeline_stages count: JSON=%d Go=%d", len(fromJSON.PipelineStages), len(fromGo.PipelineStages))
	}
	if len(fromJSON.Automations) != len(fromGo.Automations) {
		t.Errorf("automations count: JSON=%d Go=%d", len(fromJSON.Automations), len(fromGo.Automations))
	}
}
