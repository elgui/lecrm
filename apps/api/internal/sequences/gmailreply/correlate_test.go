package gmailreply

import (
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences"
)

func TestNormalizeMessageID(t *testing.T) {
	cases := map[string]string{
		"<abc@mail>":    "abc@mail",
		"  <abc@mail> ": "abc@mail",
		"abc@mail":      "abc@mail",
		"<MiXeD@Case>":  "MiXeD@Case", // case preserved (RFC 5322 local part is case-sensitive)
		"":              "",
		"<>":            "",
	}
	for in, want := range cases {
		if got := NormalizeMessageID(in); got != want {
			t.Errorf("NormalizeMessageID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReferencedMessageIDs_UnionDedupOrder(t *testing.T) {
	m := InboundMessage{
		InReplyTo:  "<direct@parent>",
		References: []string{"<root@thread> <direct@parent>", "<mid@thread>"},
	}
	got := m.ReferencedMessageIDs()
	// In-Reply-To first, then References, de-duplicated, brackets stripped.
	want := []string{"direct@parent", "root@thread", "mid@thread"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReferencedMessageIDs = %v, want %v", got, want)
	}
}

func TestCorrelateReplies_MatchWaitingReply(t *testing.T) {
	enr := uuid.New()
	msgs := []InboundMessage{{
		RFC822MessageID: "reply-1@gmail",
		InReplyTo:       "<sent-step@lecrm>",
	}}
	matched := []MatchedStep{{
		RFCMessageID: "sent-step@lecrm",
		EnrollmentID: enr,
		StepIndex:    1,
		State:        sequences.StateWaitingReply,
	}}

	out := correlateReplies(msgs, matched)
	if len(out) != 1 {
		t.Fatalf("got %d matches, want 1", len(out))
	}
	got := out[0]
	if got.EnrollmentID != enr || got.StepIndex != 1 {
		t.Errorf("enrollment/step = %s/%d", got.EnrollmentID, got.StepIndex)
	}
	if got.RFCMessageID != "sent-step@lecrm" {
		t.Errorf("RFCMessageID = %q", got.RFCMessageID)
	}
	if got.ReplyMessageID != "reply-1@gmail" {
		t.Errorf("ReplyMessageID = %q", got.ReplyMessageID)
	}
}

func TestCorrelateReplies_SkipsNonWaitingState(t *testing.T) {
	enr := uuid.New()
	msgs := []InboundMessage{{RFC822MessageID: "r@x", InReplyTo: "<sent@lecrm>"}}
	matched := []MatchedStep{{
		RFCMessageID: "sent@lecrm",
		EnrollmentID: enr,
		StepIndex:    0,
		State:        sequences.StateReplyReceived, // already moved on
	}}
	if out := correlateReplies(msgs, matched); len(out) != 0 {
		t.Fatalf("expected no matches for non-waiting enrollment, got %d", len(out))
	}
}

func TestCorrelateReplies_NoMatchUnknownReference(t *testing.T) {
	msgs := []InboundMessage{{RFC822MessageID: "r@x", References: []string{"<unknown@thread>"}}}
	matched := []MatchedStep{{RFCMessageID: "other@lecrm", EnrollmentID: uuid.New(), State: sequences.StateWaitingReply}}
	if out := correlateReplies(msgs, matched); len(out) != 0 {
		t.Fatalf("expected no matches, got %d", len(out))
	}
}

func TestCorrelateReplies_OneMatchPerEnrollment(t *testing.T) {
	enr := uuid.New()
	// Two inbound replies both referencing the same sent step — only the first
	// should transition the enrollment (no double-transition).
	msgs := []InboundMessage{
		{RFC822MessageID: "first@gmail", InReplyTo: "<sent@lecrm>"},
		{RFC822MessageID: "second@gmail", InReplyTo: "<sent@lecrm>"},
	}
	matched := []MatchedStep{{RFCMessageID: "sent@lecrm", EnrollmentID: enr, State: sequences.StateWaitingReply}}
	out := correlateReplies(msgs, matched)
	if len(out) != 1 {
		t.Fatalf("got %d matches, want 1 (first reply wins)", len(out))
	}
	if out[0].ReplyMessageID != "first@gmail" {
		t.Errorf("first reply should win, got %q", out[0].ReplyMessageID)
	}
}

func TestCollectReferencedIDs_Dedup(t *testing.T) {
	msgs := []InboundMessage{
		{InReplyTo: "<a@x>", References: []string{"<b@x>"}},
		{InReplyTo: "<b@x>", References: []string{"<c@x>"}}, // b@x duplicate across messages
	}
	got := collectReferencedIDs(msgs)
	want := []string{"a@x", "b@x", "c@x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectReferencedIDs = %v, want %v", got, want)
	}
}
