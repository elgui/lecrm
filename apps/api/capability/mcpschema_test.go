package capability

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestMCPWorkspaceSchema_CompactSerialisation(t *testing.T) {
	s := MCPWorkspaceSchema{
		WorkspaceID: uuid.MustParse("11111111-1111-1111-1111-111111111111").String(),
		Contact: []MCPPropertyDef{
			{Key: "lead_score", Type: "number"},
			{Key: "cms", Type: "enum", AllowedValues: []string{"wordpress", "shopify"}, Required: true},
		},
		Deal: []MCPPropertyDef{},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)

	// Token efficiency: an unconstrained property omits allowed_values and
	// required entirely (omitempty), so only fields that constrain it appear.
	if strings.Contains(got, `"allowed_values":null`) || strings.Contains(got, `"allowed_values":[]`) {
		t.Errorf("unconstrained prop must omit allowed_values: %s", got)
	}
	if !strings.Contains(got, `{"key":"lead_score","type":"number"}`) {
		t.Errorf("lead_score should serialise to just key+type: %s", got)
	}
	// Constrained property carries its values + required.
	if !strings.Contains(got, `"allowed_values":["wordpress","shopify"]`) || !strings.Contains(got, `"required":true`) {
		t.Errorf("enum prop must carry allowed_values + required: %s", got)
	}
	// Empty group serialises as [] (never null) — a valid, complete shape.
	if !strings.Contains(got, `"deal":[]`) {
		t.Errorf("empty group must be [] not null: %s", got)
	}
}
