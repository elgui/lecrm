package ooo

import (
	"context"
	"errors"
	"testing"
	"time"
)

// stubLLM is a test double for the stage-2 seam. It records call count so tests
// can assert the rules absorbed a case without escalating.
type stubLLM struct {
	isOOO bool
	conf  Confidence
	err   error
	calls int
}

func (s *stubLLM) ClassifyOOO(_ context.Context, _ ReplyBody) (bool, Confidence, error) {
	s.calls++
	return s.isOOO, s.conf, s.err
}

func TestEngineClassify_RulesCertainOOO(t *testing.T) {
	llm := &stubLLM{}
	e := NewEngine(WithLLM(llm), WithClock(func() time.Time { return fixedNow }))

	cat, conf, ret, err := e.Classify(context.Background(), ReplyBody{
		Subject: "Out of Office",
		Body:    "I am out of the office, de retour le 15 mai.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat != CategoryOOO {
		t.Errorf("category = %q, want %q", cat, CategoryOOO)
	}
	if conf < 0.9 {
		t.Errorf("rules-certain OOO confidence = %v, want ≥ 0.9", conf)
	}
	if !ret.Parsed {
		t.Error("expected a parsed return date")
	}
	want := time.Date(2026, time.May, 15, resumeHourUTC, 0, 0, 0, time.UTC)
	if !ret.Time.Equal(want) {
		t.Errorf("return date = %s, want %s", ret.Time.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if llm.calls != 0 {
		t.Errorf("LLM called %d times on a rules-certain OOO; want 0", llm.calls)
	}
}

func TestEngineClassify_RulesCertainReply(t *testing.T) {
	llm := &stubLLM{}
	e := NewEngine(WithLLM(llm))

	cat, _, ret, err := e.Classify(context.Background(), ReplyBody{
		Subject: "Re: Pricing",
		Body:    "Sounds good — let's talk Thursday.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat != CategoryReply {
		t.Errorf("category = %q, want %q", cat, CategoryReply)
	}
	if !ret.Time.IsZero() {
		t.Errorf("reply must carry no return date, got %s", ret.Time)
	}
	if llm.calls != 0 {
		t.Errorf("LLM called %d times on a rules-certain reply; want 0", llm.calls)
	}
}

// ambiguousBody is one medium signal (a generic auto-reply marker with no strong
// OOO phrase and no return clause) — the rules report ambiguous and escalate.
const ambiguousSubject = "Re: Your order"
const ambiguousBody = "This is an automated message. We have received your request."

func TestEngineClassify_AmbiguousNoLLM_DegradesToReply(t *testing.T) {
	e := NewEngine() // no LLM wired

	cat, conf, ret, err := e.Classify(context.Background(), ReplyBody{Subject: ambiguousSubject, Body: ambiguousBody})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat != CategoryReply {
		t.Errorf("category = %q, want conservative %q", cat, CategoryReply)
	}
	if conf != ambiguousReplyConfidence {
		t.Errorf("defaulted-reply confidence = %v, want %v", conf, ambiguousReplyConfidence)
	}
	if !ret.Time.IsZero() {
		t.Errorf("reply must carry no return date, got %s", ret.Time)
	}
}

func TestEngineClassify_AmbiguousLLMSaysOOO(t *testing.T) {
	llm := &stubLLM{isOOO: true, conf: 0.81}
	e := NewEngine(WithLLM(llm), WithClock(func() time.Time { return fixedNow }))

	cat, conf, ret, err := e.Classify(context.Background(), ReplyBody{Subject: ambiguousSubject, Body: ambiguousBody})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat != CategoryOOO {
		t.Errorf("category = %q, want %q", cat, CategoryOOO)
	}
	if conf != 0.81 {
		t.Errorf("confidence = %v, want LLM's 0.81", conf)
	}
	if llm.calls != 1 {
		t.Errorf("LLM called %d times; want exactly 1", llm.calls)
	}
	// No parseable date in the body → +5-business-day default, but still populated.
	if ret.Parsed {
		t.Error("expected the default (unparsed) return date")
	}
	if ret.Time.IsZero() {
		t.Error("OOO verdict must always carry a return date")
	}
	wantDefault := AddBusinessDays(fixedNow, defaultBusinessDays)
	if !ret.Time.Equal(wantDefault) {
		t.Errorf("default return date = %s, want %s", ret.Time.Format(time.RFC3339), wantDefault.Format(time.RFC3339))
	}
}

func TestEngineClassify_AmbiguousLLMSaysReply(t *testing.T) {
	llm := &stubLLM{isOOO: false, conf: 0.66}
	e := NewEngine(WithLLM(llm))

	cat, conf, ret, err := e.Classify(context.Background(), ReplyBody{Subject: ambiguousSubject, Body: ambiguousBody})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat != CategoryReply {
		t.Errorf("category = %q, want %q", cat, CategoryReply)
	}
	if conf != 0.66 {
		t.Errorf("confidence = %v, want LLM's 0.66", conf)
	}
	if !ret.Time.IsZero() {
		t.Errorf("reply must carry no return date, got %s", ret.Time)
	}
	if llm.calls != 1 {
		t.Errorf("LLM called %d times; want exactly 1", llm.calls)
	}
}

func TestEngineClassify_AmbiguousLLMError_FailsClosed(t *testing.T) {
	sentinel := errors.New("anthropic 529 overloaded")
	llm := &stubLLM{err: sentinel}
	e := NewEngine(WithLLM(llm))

	_, _, _, err := e.Classify(context.Background(), ReplyBody{Subject: ambiguousSubject, Body: ambiguousBody})
	if !errors.Is(err, sentinel) {
		t.Fatalf("LLM error must surface so the worker rolls back; got %v", err)
	}
}

func TestEngineClassify_SatisfiesSeam(t *testing.T) {
	var _ Classifier = NewEngine()
}
