package sequences

// Audit event names for sequences state transitions (ADR-004 rev 2 §6).
// These extend the ADR-007 §3 audit catalogue. They are the shared names
// only — the emission (one capability.EmitAudit call inside the same
// transaction as the state change, tagged with
// capability.ActorTypeInternalService) is delivered by the state-machine
// tasket (20260614-154815-ff66).
//
// Exactly the events catalogued in §6 are declared here. §6 does not
// name a distinct event for entering waiting_reply, suppressed, or
// completed; whether those transitions emit a generic event or reuse
// finalize is a decision the state-machine tasket owns. The foundation
// deliberately does not invent names the ADR has not blessed.
const (
	// AuditEventEnrolled — fields: enrollment_id, sequence_id,
	// contact_id, created_by_user_id (nullable). Retention: data (3y).
	AuditEventEnrolled = "sequences.enrolled"

	// AuditEventStepSent — fields: enrollment_id, step_index,
	// brevo_message_id, rfc_message_id. Retention: data (3y).
	AuditEventStepSent = "sequences.step_sent"

	// AuditEventReplyReceived — fields: enrollment_id, step_index,
	// reply_message_id, classifier_category, classifier_confidence,
	// detector (gmail_push | brevo_inbound). Retention: data (3y).
	AuditEventReplyReceived = "sequences.reply_received"

	// AuditEventOOODetected — AuditEventReplyReceived fields plus
	// ooo_returns_at (nullable). Retention: data (3y).
	AuditEventOOODetected = "sequences.ooo_detected"

	// AuditEventFailed — fields: enrollment_id, step_index, error,
	// attempts. Retention: data (3y).
	AuditEventFailed = "sequences.failed"

	// AuditEventBounced — fields: enrollment_id, email, bounce_type,
	// smtp_code. Retention: data (3y).
	AuditEventBounced = "sequences.bounced"

	// AuditEventUnsubscribed — fields: enrollment_id, email, source
	// (list_unsubscribe | complaint | manual). Retention: data (3y).
	AuditEventUnsubscribed = "sequences.unsubscribed"

	// AuditEventTransitionInvalid — fields: enrollment_id, from,
	// to_attempted, caller. Retention: auth (1y) — programming-error
	// trace for an illegal transition (§2).
	AuditEventTransitionInvalid = "sequences.transition.invalid"
)

// auditEvents is every declared sequences audit event name.
var auditEvents = []string{
	AuditEventEnrolled,
	AuditEventStepSent,
	AuditEventReplyReceived,
	AuditEventOOODetected,
	AuditEventFailed,
	AuditEventBounced,
	AuditEventUnsubscribed,
	AuditEventTransitionInvalid,
}

// AuditEvents returns a copy of the sequences audit event names declared
// in ADR-004 rev 2 §6. The state-machine tasket uses it to register the
// event set with the audit fixture exercised by the pen-test cadence
// (ADR-004 rev 2 §S4).
func AuditEvents() []string {
	out := make([]string, len(auditEvents))
	copy(out, auditEvents)
	return out
}
