import { useEffect, useState } from 'react';
import { Sparkles, X, Lock } from 'lucide-react';

// Per-record "Assistant IA" seat — an HONEST PLACEHOLDER, not a live bot.
//
// leCRM's differentiator is an AI-native UX that integrators wire to their own
// model because the source is theirs (AGPL). The seat for that surface lives at
// the per-record detail level (contact + deal). Until a backend exists it ships
// branded, reserved, and explicitly "not yet live" — and deliberately has NO
// text input, so a demo viewer can never "ask a second question" and watch the
// illusion collapse.
//
// FUTURE-WIRING CONTRACT (Winston's guardrail — honour all three before this
// placeholder is replaced by a live, input-bearing chat UI):
//   1. A single SSE STREAMING endpoint at `AI_CHAT_ENDPOINT` (/v1/ai/chat).
//   2. The prompt is assembled SERVER-SIDE from the authenticated workspace
//      ONLY — no cross-tenant data ever reaches the model. The client passes
//      record identifiers (kind + id), never raw record contents.
//   3. The LLM call is routed through an EU-REGION model, so the AI surface
//      inherits the same data-residency / tenant guarantees as any other read;
//      otherwise the first AI feature undercuts the EU-residency pitch.
//
// While `aiEnabled` in lib/ai.ts is false the body below renders the
// placeholder and the endpoint stays reserved (no live call is ever made here).
const AI_CHAT_ENDPOINT = '/v1/ai/chat';

type RecordKind = 'contact' | 'deal';

const RECORD_NOUN: Record<RecordKind, string> = {
  contact: 'ce contact',
  deal: 'cette affaire',
};

interface AssistantIaRailProps {
  recordKind: RecordKind;
  /** Optional record label, surfaced in the panel to make the seat feel placed. */
  recordName?: string;
}

export function AssistantIaRail({ recordKind, recordName }: AssistantIaRailProps) {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open]);

  const noun = RECORD_NOUN[recordKind];

  return (
    <>
      {/* Docked affordance — slim vertical tab on the right edge of the viewport */}
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label="Ouvrir l’Assistant IA"
        aria-expanded={open}
        className="fixed right-0 top-1/2 z-30 flex -translate-y-1/2 items-center gap-2 rounded-l-lg border border-r-0 border-primary/30 bg-primary/10 px-2 py-4 text-primary shadow-sm transition-colors hover:bg-primary/20 [writing-mode:vertical-rl]"
      >
        <Sparkles className="h-4 w-4 rotate-90" aria-hidden />
        <span className="text-xs font-medium tracking-wide">Assistant IA</span>
      </button>

      {open && (
        <>
          {/* Backdrop */}
          <div
            className="fixed inset-0 z-40 bg-foreground/20 backdrop-blur-[1px]"
            onClick={() => setOpen(false)}
            aria-hidden
          />

          {/* Docked panel */}
          <aside
            role="dialog"
            aria-modal="true"
            aria-labelledby="assistant-ia-title"
            className="fixed right-0 top-0 z-50 flex h-full w-80 flex-col border-l bg-card shadow-xl"
          >
            <header className="flex items-center justify-between border-b px-4 py-3">
              <div className="flex items-center gap-2">
                <span className="flex h-7 w-7 items-center justify-center rounded-md bg-primary/10 text-primary">
                  <Sparkles className="h-4 w-4" aria-hidden />
                </span>
                <h2 id="assistant-ia-title" className="text-sm font-semibold">
                  Assistant IA
                </h2>
                <span className="rounded-full border border-primary/30 bg-primary/5 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-primary">
                  Bientôt
                </span>
              </div>
              <button
                type="button"
                onClick={() => setOpen(false)}
                aria-label="Fermer"
                className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              >
                <X className="h-4 w-4" aria-hidden />
              </button>
            </header>

            <div className="flex flex-1 flex-col gap-4 overflow-y-auto px-4 py-5">
              <div className="rounded-lg border border-dashed border-primary/30 bg-primary/5 p-4">
                <p className="text-sm font-medium text-foreground">Bientôt disponible</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Cet espace accueillera un assistant capable de résumer {noun}
                  {recordName ? ` — ${recordName}` : ''}, de préparer vos relances et
                  de répondre à vos questions, directement dans la fiche.
                </p>
              </div>

              <div className="space-y-2 text-sm text-muted-foreground">
                <p className="font-medium text-foreground">Le siège est réservé.</p>
                <p>
                  Connectez votre propre modèle —{' '}
                  <span className="font-medium text-foreground">
                    le code est à vous (AGPL)
                  </span>
                  . Aucun fournisseur imposé, aucune donnée envoyée à un tiers par
                  défaut.
                </p>
              </div>

              <ul className="space-y-2 text-sm text-muted-foreground">
                <li className="flex gap-2">
                  <span className="mt-1.5 h-1.5 w-1.5 flex-none rounded-full bg-primary/60" />
                  <span>
                    Le prompt sera construit côté serveur, à partir de votre seul
                    espace de travail.
                  </span>
                </li>
                <li className="flex gap-2">
                  <span className="mt-1.5 h-1.5 w-1.5 flex-none rounded-full bg-primary/60" />
                  <span>
                    Appel routé via un modèle hébergé en UE — mêmes garanties de
                    résidence des données que le reste de votre CRM.
                  </span>
                </li>
              </ul>
            </div>

            <footer className="flex items-center gap-2 border-t px-4 py-3 text-xs text-muted-foreground">
              <Lock className="h-3.5 w-3.5 flex-none" aria-hidden />
              <span>Non connecté · point d’entrée réservé {AI_CHAT_ENDPOINT}</span>
            </footer>
          </aside>
        </>
      )}
    </>
  );
}
