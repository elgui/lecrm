package crm

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gbconsult/lecrm/apps/api/internal/sqlcgen"
)

// =========================================================================
// validateEntityType
// =========================================================================

func TestValidateEntityType_AcceptsKnown(t *testing.T) {
	for _, ok := range []string{entityTypeContact, entityTypeCompany, entityTypeDeal} {
		if !validateEntityType(ok) {
			t.Errorf("validateEntityType(%q) = false, want true", ok)
		}
	}
}

func TestValidateEntityType_RejectsUnknown(t *testing.T) {
	for _, bad := range []string{"", "user", "pipeline", "Contact", "DEAL", "note"} {
		if validateEntityType(bad) {
			t.Errorf("validateEntityType(%q) = true, want false", bad)
		}
	}
}

// =========================================================================
// activityFromRow
// =========================================================================

func TestActivityFromRow_DefaultPayloadIsEmptyObject(t *testing.T) {
	row := sqlcgen.Activity{
		ID: uuid.New(), EntityType: entityTypeContact, EntityID: uuid.New(),
		EventType: "entity.created",
		Payload:   nil,
	}
	got := activityFromRow(row)
	if string(got.Payload) != "{}" {
		t.Errorf("nil payload should marshal as {}; got %q", got.Payload)
	}
}

func TestActivityFromRow_PassesThroughPayloadBytes(t *testing.T) {
	want := `{"key":"value","n":42}`
	row := sqlcgen.Activity{
		ID:        uuid.New(),
		EventType: "x",
		Payload:   []byte(want),
	}
	got := activityFromRow(row)
	// Ensure it survives a JSON encode round-trip.
	b, err := json.Marshal(got.Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != want {
		t.Errorf("payload round-trip: got %s want %s", b, want)
	}
}

func TestActivityFromRow_PopulatesActorAndSource(t *testing.T) {
	actor := "connector"
	src := "chatboting"
	row := sqlcgen.Activity{
		ID:           uuid.New(),
		EntityType:   entityTypeDeal,
		EventType:    "deal.stage_changed",
		ActorType:    pgtype.Text{String: actor, Valid: true},
		SourceSystem: pgtype.Text{String: src, Valid: true},
	}
	got := activityFromRow(row)
	if got.ActorType == nil || *got.ActorType != actor {
		t.Errorf("ActorType: got %v want %q", got.ActorType, actor)
	}
	if got.SourceSystem == nil || *got.SourceSystem != src {
		t.Errorf("SourceSystem: got %v want %q", got.SourceSystem, src)
	}
}

// =========================================================================
// noteFromRow / taskFromRow basic shape preservation
// =========================================================================

func TestNoteFromRow_PreservesAuthorAndBody(t *testing.T) {
	author := uuid.New()
	row := sqlcgen.Note{
		ID: uuid.New(), EntityType: entityTypeContact, EntityID: uuid.New(),
		Body: "hello", AuthorID: author,
	}
	got := noteFromRow(row)
	if got.AuthorID != author {
		t.Errorf("author: got %v want %v", got.AuthorID, author)
	}
	if got.Body != "hello" {
		t.Errorf("body: got %q", got.Body)
	}
}

func TestTaskFromRow_OptionalFieldsRoundTripAsNil(t *testing.T) {
	row := sqlcgen.Task{
		ID: uuid.New(), Title: "T",
	}
	got := taskFromRow(row)
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt: want nil got %v", got.CompletedAt)
	}
	if got.DueDate != nil {
		t.Errorf("DueDate: want nil got %v", got.DueDate)
	}
	if got.AssigneeID != nil {
		t.Errorf("AssigneeID: want nil got %v", got.AssigneeID)
	}
}
