// Package ooo is the two-stage out-of-office classifier (ADR-004 rev 2 §5).
//
// A detected reply is either a genuine human reply (→ reply_received) or an
// auto-responder (→ ooo_detected, resume after the return date). Stage 1 is a
// French + English regex rule set that catches ~95% of out-of-office replies at
// zero cost and high precision (§Q1). Stage 2 is an optional Haiku fallback for
// the ambiguous tail (cases the rules cannot call confidently either way).
//
// The whole package hangs off a single one-method seam:
//
//	Classify(ctx, ReplyBody) (Category, Confidence, OOOReturnDate, error)
//
// ADR-004 rev 2 §5 names the classifier "the most-likely-to-be-rewritten module
// in v2 (FastText, fine-tuned small model, etc.)"; keeping the coupling to one
// method is what makes a swap not touch the state machine.
package ooo

import (
	"context"
	"time"
)

// Category is the classifier verdict for one inbound reply. The string values
// are recorded verbatim in the sequences.reply_received / sequences.ooo_detected
// audit payloads (ADR-004 rev 2 §6, classifier_category field).
type Category string

const (
	// CategoryReply is a genuine human reply — routes the enrollment to
	// reply_received (terminal in v1).
	CategoryReply Category = "reply"
	// CategoryOOO is an out-of-office / vacation auto-responder — routes the
	// enrollment to ooo_detected and reschedules at the return date.
	CategoryOOO Category = "ooo"
)

// Confidence is the classifier's confidence in its verdict, in [0,1]. It is
// surfaced as classifier_confidence in the audit payload (§6); it is advisory
// for reporting and the rules-vs-ML reconsideration trigger (§Q1), never a gate
// on the transition itself.
type Confidence float64

// ReplyBody is the minimised view of an inbound reply the classifier reasons
// over. Per ADR-009 §8.3 the full body is never carried on in-flight payloads —
// the sender, subject, and a short snippet are enough for both the rules and the
// Haiku fallback, and keep PII off the worker's transaction.
type ReplyBody struct {
	From    string // From header (sender; e.g. "Jane <jane@acme.fr>")
	Subject string // Subject header
	Body    string // short snippet / preview text (NOT the full body)
}

// OOOReturnDate is when an out-of-office enrollment should resume. When the
// verdict is CategoryOOO, Time is always set — either extracted from the reply
// (Parsed == true) or the +5-business-day default (Parsed == false, ADR-004
// rev 2 §5 / rev 1). For CategoryReply it is the zero value.
type OOOReturnDate struct {
	Time   time.Time
	Parsed bool
}

// Classifier is the one-method seam between the OOO classifier and the
// enrollment state machine (ADR-004 rev 2 §5). The Gmail/Brevo reply workers
// depend only on this; *Engine is the v1 rules+Haiku implementation.
type Classifier interface {
	Classify(ctx context.Context, body ReplyBody) (Category, Confidence, OOOReturnDate, error)
}

// LLMClassifier is the stage-2 fallback seam: it decides OOO-vs-reply for the
// ambiguous tail the rules cannot call. *HaikuClassifier satisfies it; tests
// substitute a stub. A nil LLMClassifier means "rules only" — ambiguous replies
// then degrade to the conservative default (see Engine.Classify).
type LLMClassifier interface {
	// ClassifyOOO reports whether the reply is an out-of-office auto-responder.
	ClassifyOOO(ctx context.Context, body ReplyBody) (isOOO bool, confidence Confidence, err error)
}

// defaultBusinessDays is the reschedule horizon when an OOO reply has no
// parseable return date (ADR-004 rev 2 §5: "Unparseable dates → reschedule
// +5 business days per rev 1 default").
const defaultBusinessDays = 5

// ambiguousReplyConfidence is the confidence attached to an ambiguous reply that
// degrades to CategoryReply because no LLM stage is wired (or the LLM declined
// OOO). It is deliberately low so reporting can distinguish a rules-certain reply
// (~0.96) from a defaulted one.
const ambiguousReplyConfidence Confidence = 0.5

// Engine is the v1 two-stage classifier (ADR-004 rev 2 §5). It owns the rule set,
// the optional Haiku fallback, and return-date extraction. It is safe for
// concurrent use: the compiled rules are immutable and Classify holds no state.
type Engine struct {
	rules *Rules
	llm   LLMClassifier
	now   func() time.Time
}

// Option configures an Engine.
type Option func(*Engine)

// WithLLM wires the stage-2 fallback used for ambiguous replies. Omit it to run
// rules-only (ambiguous → conservative CategoryReply).
func WithLLM(llm LLMClassifier) Option {
	return func(e *Engine) { e.llm = llm }
}

// WithClock overrides the time source (return-date year inference and the
// +5-business-day default). Defaults to time.Now. Tests pin it for determinism.
func WithClock(now func() time.Time) Option {
	return func(e *Engine) {
		if now != nil {
			e.now = now
		}
	}
}

// NewEngine builds the two-stage classifier with the frozen rule set and the
// given options.
func NewEngine(opts ...Option) *Engine {
	e := &Engine{
		rules: compiledRules,
		now:   time.Now,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Classify runs stage 1 (rules), escalating only the ambiguous tail to stage 2
// (Haiku). It is the sole coupling between this package and the state machine.
//
//   - A rules-certain OOO or reply returns immediately (the ~95% zero-cost path).
//   - An ambiguous reply goes to the LLM if one is wired; a wired LLM that errors
//     surfaces the error so the worker rolls back and river retries (fail-closed,
//     never silently guess on a tool outage).
//   - With no LLM wired, an ambiguous reply degrades to CategoryReply — the same
//     conservative bias as the foundation's DefaultClassifier: misreading an OOO
//     as a real reply only ends the sequence early; it never keeps emailing
//     someone who engaged.
func (e *Engine) Classify(ctx context.Context, body ReplyBody) (Category, Confidence, OOOReturnDate, error) {
	v := e.rules.Evaluate(body)
	switch v.verdict {
	case verdictOOO:
		return CategoryOOO, v.confidence, e.returnDate(body), nil
	case verdictReply:
		return CategoryReply, v.confidence, OOOReturnDate{}, nil
	default: // verdictAmbiguous
		if e.llm == nil {
			return CategoryReply, ambiguousReplyConfidence, OOOReturnDate{}, nil
		}
		isOOO, conf, err := e.llm.ClassifyOOO(ctx, body)
		if err != nil {
			return CategoryReply, 0, OOOReturnDate{}, err
		}
		if isOOO {
			return CategoryOOO, conf, e.returnDate(body), nil
		}
		return CategoryReply, conf, OOOReturnDate{}, nil
	}
}

// returnDate extracts the OOO return date from the reply, falling back to
// +defaultBusinessDays business days when none parses (ADR-004 rev 2 §5).
func (e *Engine) returnDate(body ReplyBody) OOOReturnDate {
	now := e.now()
	if t, ok := ParseReturnDate(body, now); ok {
		return OOOReturnDate{Time: t, Parsed: true}
	}
	return OOOReturnDate{Time: AddBusinessDays(now, defaultBusinessDays), Parsed: false}
}

// Compile-time proof the engine satisfies the public seam.
var _ Classifier = (*Engine)(nil)
