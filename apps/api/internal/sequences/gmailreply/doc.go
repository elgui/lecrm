// Package gmailreply is the Gmail-path reply detection for the v1 native
// sequences engine (ADR-004 rev 2 §4, tasket 20260614-154815-5b07). It turns
// inbound Gmail events into enrollment state transitions:
//
//	Gmail users.watch() ──▶ Pub/Sub topic gmail-inbox-events
//	                          │ push (OIDC-signed)
//	                          ▼
//	  POST /v1/webhooks/gmail/push   ── PushHandler (this package)
//	     1. validate the Google-signed OIDC JWT  (TokenValidator)
//	     2. resolve emailAddress → workspace+user (ConnectionResolver)
//	     3. enqueue sequences.gmail.poll_mailbox  (MailboxPollEnqueuer)
//	                          │
//	                          ▼
//	  poll_mailbox worker (workspace-scoped tx)
//	     4. users.history.list(startHistoryId=last_history_id)  (HistoryClient)
//	     5. extract In-Reply-To / References from each new INBOX message
//	     6. one indexed batch match against enrollment_steps.rfc_message_id
//	     7. Classify (rules+Haiku, order:7) → reply_received | ooo_detected
//	     8. sequences.Transition(waiting_reply → …) + persist new history_id
//
// Daily, the gmail.watch_renew periodic job (cron "0 4 * * *", PeriodicWatchRenew)
// re-registers the Gmail watch — a watch expires after 7 days, so renewing
// daily keeps a wide safety margin and expiry never drops detection.
//
// # Why a Gmail-specific mailbox-scan job (not the per-enrollment poll_reply)
//
// A Pub/Sub push only says "mailbox X has new history"; it carries no
// enrollment id. ADR-004 rev 2 §4's correlation logic is therefore
// mailbox-level: ONE history.list and ONE indexed batch query that matches
// every new message's referenced Message-IDs against enrollment_steps in a
// single round trip. That maps to a mailbox-scoped job (PollMailboxArgs,
// keyed on workspace+user), distinct from the foundation's per-enrollment
// sequences.poll_reply (which remains the reply-window timeout path). Keeping
// it here leaves the foundation's four job kinds (sequences/jobs.go) and the
// PollReplyArgs payload untouched.
//
// # Seams (what is real here vs injected)
//
// The package logic — push-envelope parsing, JWT-claim verification, reply
// correlation, the indexed match query, cursor persistence, and the worker
// orchestration — is real and unit-tested. The two boundaries that need live
// Google access are interfaces with thin production implementations:
//
//   - TokenValidator   → GoogleTokenValidator (google.golang.org/api/idtoken)
//   - HistoryClient /  → googleClient (google.golang.org/api/gmail/v1 +
//     ClientFactory       golang.org/x/oauth2/google), built from the
//     /Classifier         SOPS-stored refresh token (ADR-007 §2).
//
// The Classifier seam is filled by the OOO classifier tasket (order:7,
// 20260614-154815-a81e); until then DefaultClassifier treats every reply as a
// genuine human reply (→ reply_received).
//
// Authoritative design: docs/adr/ADR-004-rev2-sequences-architecture.md §4;
// secret storage: ADR-007 §2; setup runbook: ops/runbooks/gmail-oauth-pubsub-setup.md.
package gmailreply
