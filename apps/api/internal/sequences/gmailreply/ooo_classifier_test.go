package gmailreply

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences/ooo"
)

// stubOOO is a test double for the ooo.Classifier seam, letting the adapter test
// drive each verdict shape without the real engine.
type stubOOO struct {
	cat  ooo.Category
	conf ooo.Confidence
	ret  ooo.OOOReturnDate
	err  error
}

func (s stubOOO) Classify(_ context.Context, _ ooo.ReplyBody) (ooo.Category, ooo.Confidence, ooo.OOOReturnDate, error) {
	return s.cat, s.conf, s.ret, s.err
}

func TestOOOAdapter_ReplyVerdict(t *testing.T) {
	a := NewClassifier(stubOOO{cat: ooo.CategoryReply, conf: 0.96})
	cls, err := a.Classify(context.Background(), InboundMessage{From: "a@b.test", Subject: "Re: x", Snippet: "ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cls.IsOOO {
		t.Error("reply verdict must not be OOO")
	}
	if cls.OOOReturnsAt != nil {
		t.Errorf("reply verdict must leave OOOReturnsAt nil, got %v", cls.OOOReturnsAt)
	}
	if cls.Category != "reply" {
		t.Errorf("category = %q, want %q", cls.Category, "reply")
	}
	if cls.Confidence != 0.96 {
		t.Errorf("confidence = %v, want 0.96", cls.Confidence)
	}
}

func TestOOOAdapter_OOOVerdictPopulatesReturnsAt(t *testing.T) {
	when := time.Date(2026, time.June, 20, 9, 0, 0, 0, time.UTC)
	a := NewClassifier(stubOOO{cat: ooo.CategoryOOO, conf: 0.97, ret: ooo.OOOReturnDate{Time: when, Parsed: true}})

	cls, err := a.Classify(context.Background(), InboundMessage{From: "a@b.test", Subject: "Out of Office", Snippet: "away"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cls.IsOOO {
		t.Error("OOO verdict must set IsOOO")
	}
	if cls.OOOReturnsAt == nil {
		t.Fatal("OOO verdict must populate OOOReturnsAt")
	}
	if !cls.OOOReturnsAt.Equal(when) {
		t.Errorf("OOOReturnsAt = %s, want %s", cls.OOOReturnsAt, when)
	}
	// The adapter must copy the value, not alias the classifier's struct field.
	*cls.OOOReturnsAt = cls.OOOReturnsAt.Add(time.Hour)
	if !when.Equal(time.Date(2026, time.June, 20, 9, 0, 0, 0, time.UTC)) {
		t.Error("mutating the returned pointer leaked back into the source date")
	}
}

func TestOOOAdapter_ErrorPropagates(t *testing.T) {
	sentinel := errors.New("classifier down")
	a := NewClassifier(stubOOO{err: sentinel})
	_, err := a.Classify(context.Background(), InboundMessage{From: "a@b.test"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("adapter must surface the classifier error, got %v", err)
	}
}
