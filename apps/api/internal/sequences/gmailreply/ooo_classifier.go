package gmailreply

import (
	"context"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences/ooo"
)

// oooAdapter adapts the ooo package's one-method classifier (ADR-004 rev 2 §5)
// to this package's Classifier seam. It is the ONLY coupling between the OOO
// classifier and the reply-detection path: it maps an InboundMessage to the
// minimised ooo.ReplyBody and the verdict back to a Classification the worker's
// applyReply already knows how to route (reply_received vs ooo_detected +
// ooo_returns_at). Swapping the classifier implementation (FastText, a fine-tuned
// model, …) never touches this adapter or the state machine.
type oooAdapter struct {
	inner ooo.Classifier
}

// NewClassifier wraps an ooo.Classifier as a gmailreply.Classifier. The wiring
// layer builds the inner engine — typically ooo.NewEngine(ooo.WithLLM(haiku)) —
// and passes the result as Deps.Classifier. With no LLM wired the engine runs
// rules-only and degrades ambiguous replies to a conservative "reply" verdict.
func NewClassifier(inner ooo.Classifier) Classifier {
	return oooAdapter{inner: inner}
}

// Classify runs the OOO classifier over the inbound reply. Only From/Subject/the
// snippet cross the boundary (ADR-009 §8.3 — the full body is never carried). A
// CategoryOOO verdict sets IsOOO and OOOReturnsAt (always populated for OOO:
// parsed from the reply, or the +5-business-day default); a CategoryReply verdict
// leaves OOOReturnsAt nil.
func (a oooAdapter) Classify(ctx context.Context, msg InboundMessage) (Classification, error) {
	cat, conf, ret, err := a.inner.Classify(ctx, ooo.ReplyBody{
		From:    msg.From,
		Subject: msg.Subject,
		Body:    msg.Snippet,
	})
	if err != nil {
		return Classification{}, err
	}

	cls := Classification{
		Category:   string(cat),
		Confidence: float64(conf),
		IsOOO:      cat == ooo.CategoryOOO,
	}
	if cls.IsOOO {
		returnsAt := ret.Time
		cls.OOOReturnsAt = &returnsAt
	}
	return cls, nil
}

// Compile-time proof the adapter satisfies this package's Classifier seam.
var _ Classifier = oooAdapter{}
