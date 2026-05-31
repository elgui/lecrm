import * as React from 'react';
import { createRoute, useNavigate } from '@tanstack/react-router';
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import { sortableKeyboardCoordinates } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

import { Route as rootRoute } from '../__root';
import { useAuth } from '@/hooks/use-auth';
import { useDeals, useTransitionDealStage } from '@/hooks/use-deals';
import { usePipelineStages, type PipelineStage } from '@/hooks/use-pipeline-stages';
import { useCompanyMap } from '@/hooks/use-companies';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { formatDateShort } from '@/lib/format';
import type { Deal } from '@/lib/types';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/pipeline/$workspaceId',
  component: PipelinePage,
});

function PipelinePage() {
  const { workspaceId } = Route.useParams();
  const auth = useAuth();
  const stagesQuery = usePipelineStages();
  const dealsQuery = useDeals();
  const transition = useTransitionDealStage();
  const companyById = useCompanyMap();
  const [mutationError, setMutationError] = React.useState<string | null>(null);

  const workspaceMismatch =
    !!auth.user && !!workspaceId && auth.user.workspace_id !== workspaceId;

  return (
    <div className="p-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight">Pipeline</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Glissez-déposez les affaires entre les étapes pour mettre à jour leur statut.
        </p>
      </div>

      {auth.isLoading && (
        <div className="space-y-3">
          <Skeleton className="h-10 w-64" />
          <Skeleton className="h-96 w-full" />
        </div>
      )}

      {workspaceMismatch && (
        <Card>
          <CardContent className="py-8 text-center">
            <p className="text-destructive">
              Espace de travail incorrect — cette URL concerne un autre espace
              que celui auquel vous êtes connecté.
            </p>
          </CardContent>
        </Card>
      )}

      {!auth.isLoading && !workspaceMismatch && (
        <PipelineBoardWithRouter
          stagesQuery={stagesQuery}
          dealsQuery={dealsQuery}
          companyById={companyById}
          onTransition={(id, stage_id) => {
            setMutationError(null);
            transition.mutate(
              { id, stage_id },
              {
                onError: (err) =>
                  setMutationError(
                    err?.message ?? 'Échec de l’enregistrement du déplacement',
                  ),
              },
            );
          }}
          mutationError={mutationError}
          onDismissError={() => setMutationError(null)}
        />
      )}
    </div>
  );
}

interface PipelineBoardWithRouterProps {
  stagesQuery: ReturnType<typeof usePipelineStages>;
  dealsQuery: ReturnType<typeof useDeals>;
  companyById: Map<string, string>;
  onTransition: (id: string, stage_id: string) => void;
  mutationError: string | null;
  onDismissError: () => void;
}

function PipelineBoardWithRouter(props: PipelineBoardWithRouterProps) {
  const navigate = useNavigate();
  return (
    <PipelineBoard
      {...props}
      onCardClick={(dealId) =>
        navigate({ to: '/deals/$dealId', params: { dealId } })
      }
    />
  );
}

export interface PipelineBoardProps {
  stagesQuery: ReturnType<typeof usePipelineStages>;
  dealsQuery: ReturnType<typeof useDeals>;
  /** id → name lookup so cards can show the company, not the raw UUID. */
  companyById?: Map<string, string>;
  onTransition: (id: string, stage_id: string) => void;
  onCardClick: (dealId: string) => void;
  mutationError: string | null;
  onDismissError: () => void;
}

export function PipelineBoard({
  stagesQuery,
  dealsQuery,
  companyById,
  onTransition,
  onCardClick,
  mutationError,
  onDismissError,
}: PipelineBoardProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  // Horizontal-scroll affordance: fade the board's left/right edges when more
  // columns sit off-screen, so the rightmost stage stays discoverable on a
  // desktop that can't fit every stage at once.
  const scrollRef = React.useRef<HTMLDivElement>(null);
  const [scrollEdges, setScrollEdges] = React.useState({ left: false, right: false });
  const updateScrollEdges = React.useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const { scrollLeft, scrollWidth, clientWidth } = el;
    setScrollEdges({
      left: scrollLeft > 4,
      right: scrollLeft + clientWidth < scrollWidth - 4,
    });
  }, []);
  React.useEffect(() => {
    updateScrollEdges();
    window.addEventListener('resize', updateScrollEdges);
    return () => window.removeEventListener('resize', updateScrollEdges);
    // Re-measure once the board mounts with real data (deps change on load).
  }, [updateScrollEdges, stagesQuery.data, dealsQuery.data]);

  if (stagesQuery.isLoading || dealsQuery.isLoading) {
    return (
      <div className="space-y-4" data-testid="pipeline-loading">
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }

  if (stagesQuery.error || !stagesQuery.data) {
    return (
      <Card>
        <CardContent className="py-16 text-center">
          <p className="text-destructive">
            Impossible de charger les étapes du pipeline
            {stagesQuery.error ? ` : ${stagesQuery.error.message}` : '.'}
          </p>
        </CardContent>
      </Card>
    );
  }

  const stages = stagesQuery.data.data;
  const deals = dealsQuery.data?.data ?? [];

  const dealsByStage = new Map<string, Deal[]>();
  for (const s of stages) dealsByStage.set(s.id, []);
  for (const d of deals) {
    if (d.stage_id && dealsByStage.has(d.stage_id)) {
      dealsByStage.get(d.stage_id)!.push(d);
    }
  }

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (!over) return;
    const dealId = String(active.id);
    const targetStageId = String(over.id);
    const deal = deals.find((d) => d.id === dealId);
    if (!deal || deal.stage_id === targetStageId) return;
    onTransition(dealId, targetStageId);
  }

  return (
    <div className="space-y-3">
      {mutationError && (
        <div
          role="alert"
          className="flex items-center justify-between rounded-md border border-destructive/50 bg-destructive/5 px-3 py-2 text-sm text-destructive"
        >
          <span>Échec de l’enregistrement du déplacement : {mutationError}</span>
          <button
            type="button"
            onClick={onDismissError}
            className="ml-4 underline hover:no-underline"
          >
            fermer
          </button>
        </div>
      )}
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={handleDragEnd}
      >
        <div className="relative">
          <div
            aria-hidden
            className={cn(
              'pointer-events-none absolute inset-y-0 left-0 z-10 w-8 bg-gradient-to-r from-background to-transparent transition-opacity duration-200',
              scrollEdges.left ? 'opacity-100' : 'opacity-0',
            )}
          />
          <div
            aria-hidden
            className={cn(
              'pointer-events-none absolute inset-y-0 right-0 z-10 w-12 bg-gradient-to-l from-background to-transparent transition-opacity duration-200',
              scrollEdges.right ? 'opacity-100' : 'opacity-0',
            )}
          />
          <div
            ref={scrollRef}
            onScroll={updateScrollEdges}
            className="flex gap-4 overflow-x-auto pb-4"
            data-testid="pipeline-board"
          >
            {stages.map((stage) => (
              <StageColumn
                key={stage.id}
                stage={stage}
                deals={dealsByStage.get(stage.id) ?? []}
                companyById={companyById}
                onCardClick={onCardClick}
              />
            ))}
          </div>
        </div>
      </DndContext>
    </div>
  );
}

interface StageColumnProps {
  stage: PipelineStage;
  deals: Deal[];
  companyById?: Map<string, string>;
  onCardClick: (dealId: string) => void;
}

function StageColumn({ stage, deals, companyById, onCardClick }: StageColumnProps) {
  const { isOver, setNodeRef } = useDroppable({ id: stage.id });

  // Summed value of the deals in this stage, formatted in the stage's own
  // currency (falling back to EUR for the French-market demo).
  const total = deals.reduce((sum, d) => sum + (d.amount ?? 0), 0);
  const currency = deals.find((d) => d.currency)?.currency ?? 'EUR';
  const totalLabel = total > 0 ? formatCurrency(total, currency) : null;

  return (
    <div
      ref={setNodeRef}
      data-testid={`pipeline-column-${stage.id}`}
      data-stage-name={stage.name}
      className={cn(
        'flex w-72 shrink-0 flex-col rounded-lg border border-border bg-muted/50 p-3 transition-colors',
        isOver && 'border-primary/50 bg-primary/5 ring-2 ring-primary/20',
      )}
    >
      <div className="mb-3 px-1">
        <div className="flex items-center justify-between">
          <h2 className="text-[13px] font-semibold uppercase tracking-wide text-muted-foreground">
            {stage.name}
          </h2>
          <span className="flex h-5 min-w-5 items-center justify-center rounded-full bg-background px-1.5 text-xs font-medium text-muted-foreground">
            {deals.length}
          </span>
        </div>
        {totalLabel && (
          <p className="mt-1 text-xs font-semibold tabular-nums text-foreground">
            {totalLabel}
          </p>
        )}
      </div>
      <div className="flex flex-col gap-2">
        {deals.length === 0 && (
          <p className="rounded-md border border-dashed border-border py-8 text-center text-xs text-muted-foreground">
            Aucune affaire
          </p>
        )}
        {deals.map((deal) => (
          <DealCard
            key={deal.id}
            deal={deal}
            companyName={
              deal.company_id ? (companyById?.get(deal.company_id) ?? null) : null
            }
            onClick={() => onCardClick(deal.id)}
          />
        ))}
      </div>
    </div>
  );
}

interface DealCardProps {
  deal: Deal;
  companyName?: string | null;
  onClick: () => void;
}

function formatCurrency(amount: number | null, currency: string | null) {
  if (amount === null || !currency) return null;
  try {
    return new Intl.NumberFormat('fr-FR', {
      style: 'currency',
      currency,
    }).format(amount);
  } catch {
    return `${amount} ${currency}`;
  }
}

function isOverdue(deal: Deal): boolean {
  if (!deal.expected_close_date || deal.closed_at) return false;
  const due = new Date(deal.expected_close_date + 'T00:00:00');
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return due.getTime() < today.getTime();
}

// True when an open deal is due within the next 14 days (but not yet overdue) —
// drives the amber "closing soon" cue on the card.
function isClosingSoon(deal: Deal): boolean {
  if (!deal.expected_close_date || deal.closed_at) return false;
  const due = new Date(deal.expected_close_date + 'T00:00:00');
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const diffDays = Math.round((due.getTime() - today.getTime()) / 86_400_000);
  return diffDays >= 0 && diffDays <= 14;
}

function DealCard({ deal, companyName, onClick }: DealCardProps) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: deal.id,
  });
  const style: React.CSSProperties = {
    transform: CSS.Translate.toString(transform),
    opacity: isDragging ? 0.4 : 1,
  };
  const overdue = isOverdue(deal);
  const closingSoon = !overdue && isClosingSoon(deal);
  const amount = formatCurrency(deal.amount, deal.currency);

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...listeners}
      {...attributes}
      data-testid={`pipeline-card-${deal.id}`}
      onClick={(e) => {
        // Only fire navigate on plain click — drag activates after 4px.
        if (!isDragging) onClick();
        e.stopPropagation();
      }}
      role="button"
      tabIndex={0}
      className={cn(
        'cursor-grab rounded-lg border border-border bg-card p-3 text-left shadow-xs transition-all hover:border-primary/30 hover:shadow-card-hover active:cursor-grabbing',
        overdue && 'border-destructive/40',
      )}
    >
      <p className="line-clamp-2 text-sm font-medium text-foreground">{deal.title}</p>
      {companyName && (
        <p className="mt-0.5 truncate text-sm text-muted-foreground">{companyName}</p>
      )}
      <div className="mt-2 flex items-center justify-between text-xs text-muted-foreground">
        <span className="font-medium tabular-nums text-foreground">{amount ?? '—'}</span>
        {deal.expected_close_date && (
          <span
            className={cn(
              'inline-flex items-center gap-1 tabular-nums',
              overdue && 'font-medium text-destructive',
              closingSoon && 'font-medium text-amber-600',
            )}
          >
            {(overdue || closingSoon) && (
              <span
                aria-hidden
                className={cn(
                  'h-1.5 w-1.5 rounded-full',
                  overdue ? 'bg-destructive' : 'bg-amber-500',
                )}
              />
            )}
            {overdue ? 'En retard · ' : ''}
            {formatDateShort(deal.expected_close_date)}
          </span>
        )}
      </div>
    </div>
  );
}
