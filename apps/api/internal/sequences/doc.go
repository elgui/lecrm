// Package sequences is the foundation contract for the leCRM v1 native
// sequences engine (Track F). It declares the shared vocabulary the
// engine is built around — the enrollment State type, the legal
// state-transition table, the four river job kinds, and the audit event
// names — but contains NO runtime behavior. The behavior is delivered by
// the sub-taskets of plan-group lecrm-v1-build, each of which builds on
// the constants and tables declared here.
//
// Authoritative design: docs/adr/ADR-004-rev2-sequences-architecture.md
// (referred to below as "ADR-004 rev 2"). This package is the umbrella
// tasket's deliverable: a single source of truth for the names and the
// legal-transition graph, so the sub-taskets do not each re-derive them.
//
// # ADR-004 rev 2 section → sub-tasket map
//
//	§1  Schema (enrollments, enrollment_steps, partial unique index)
//	      → tasket 20260614-154749-c718 — "Sequences schema + actor_type migration"
//	        (also lands the audit_log.actor_type pre-req column, §Q6 / S1)
//	§3  Four river job types, tenant-scoped (ADR-009 §8.3)
//	      → tasket 20260614-154815-2133 — "river job framework + workers"
//	        (consumes JobKind* constants declared in jobs.go)
//	§2  State machine + Transition(ctx, tx, enrollmentID, to, reason)
//	§6  Audit emission in the same transaction, with actor_type
//	      → tasket 20260614-154815-ff66 — "state machine + Transition() + audit"
//	        (consumes the State type, legalTransitions table, and
//	         AuditEvent* names declared in state.go / audit.go)
//	§4  Reply detection — Gmail Pub/Sub Watch (OAuth + GCP human setup)
//	      → tasket 20260614-154815-5078 — "Gmail Pub/Sub — human setup checkpoint"
//	§4  Gmail push handler + sequences.poll_reply worker
//	      → tasket 20260614-154815-5b07 — "Gmail push handler + poll_reply worker"
//	§5  OOO classifier (rules + Haiku), package ooo/
//	      → tasket 20260614-154815-a81e — "OOO classifier (rules + Haiku)"
//	§8  Pre-flight: suppression + volume caps + throttle + GlockApps scoring
//	      → tasket 20260614-154815-d8f9 — "Preflight ..."
//
// # What lives here (foundation) vs in the sub-taskets (behavior)
//
//   - state.go declares the State enum (mirrors the Postgres
//     enrollment_state ENUM in §1), the terminal-state set, and the
//     legalTransitions table as pure data. It exposes read-only
//     predicates (Valid, IsTerminal, CanTransition, AllowedTransitions).
//     It deliberately does NOT implement Transition() — the row lock,
//     UPDATE, in-transaction audit emission, and next-job enqueue are
//     the state-machine tasket's job (ff66), written against this table.
//   - jobs.go declares the four JobKind constants (§3). The river worker
//     registration, args structs, UniqueOpts wiring, and handler bodies
//     belong to the river-framework tasket (2133) and the per-worker
//     taskets.
//   - audit.go declares the sequences.* audit event names (§6). The
//     emission (capability.EmitAudit inside the transition tx, with the
//     actor_type claim) belongs to the state-machine tasket (ff66).
//
// Actor attribution for engine-emitted transitions is
// capability.ActorTypeInternalService per ADR-004 rev 2 §6 / ADR-009
// §4.1; the constants live in apps/api/capability and are intentionally
// not duplicated here.
package sequences
