// Vercel AI SDK 6 wiring — Sprint 9.
//
// leCRM v0 ships NO AI-facing features. This module exists only to lock
// the dependency in (`ai` + `@ai-sdk/react`) and prove the chat/voice
// optionality is preserved without a frontend rewrite at v2 (ADR-009 —
// AI-native interfaces are the strategic moat). Nothing here is mounted
// in the running app yet; importing it has no runtime effect beyond
// holding a reference so tree-shaking does not drop the dependency.
//
// When v2 turns on assistant features, point `AI_TRANSPORT` at the
// (future) /v1/ai/chat endpoint and feed it to `useChat` from
// `@ai-sdk/react`. Until then the export is intentionally inert.

import { DefaultChatTransport } from 'ai';

/**
 * AI_CHAT_PATH is the workspace-scoped endpoint a future assistant would
 * stream from. Reserved now; not wired to any handler at v0.
 */
export const AI_CHAT_PATH = '/v1/ai/chat' as const;

/**
 * aiTransport is a pre-configured transport kept ready for v2. It is not
 * referenced by any mounted component at v0 — see the module comment.
 */
export const aiTransport = new DefaultChatTransport({ api: AI_CHAT_PATH });

/** aiEnabled is the v0 feature flag for AI surfaces — always false today. */
export const aiEnabled = false as const;
