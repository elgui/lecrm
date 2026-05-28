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
import { CircleDollarSign } from 'lucide-react';

import { Route as rootRoute } from '../__root';
import { useAuth } from '@/hooks/use-auth';
import { useDeals, useTransitionDealStage } from '@/hooks/use-deals';
import { usePipelineStages, type PipelineStage } from '@/hooks/use-pipeline-stages';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
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
  const [mutationError, setMutationError] = React.useState<string | null>(null);

  const workspaceMismatch =
    !!auth.user && !!workspaceId && auth.user.workspace_id !== workspaceId;

  return (
    <div className="p-8">
      <div className="mb-6 flex items-center gap-3">
        <CircleDollarSign className="h-6 w-6 text-muted-foreground" />
        <h1 className="text-2xl font-semibold">Pipeline</h1>
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
              Workspace mismatch — this URL is for a different workspace
              than you are signed into.
            </p>
          </CardContent>
        </Card>
      )}

      {!auth.isLoading && !workspaceMismatch && (
        <PipelineBoardWithRouter
          stagesQuery={stagesQuery}
          dealsQuery={dealsQuery}
          onTransition={(id, stage_id) => {
            setMutationError(null);
            transition.mutate(
              { id, stage_id },
              {
                onError: (err) =>
                  setMutationError(err?.message ?? 'Could not save move'),
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
  onTransition: (id: string, stage_id: string) => void;
  onCardClick: (dealId: string) => void;
  mutationError: string | null;
  onDismissError: () => void;
}

export function PipelineBoard({
  stagesQuery,
  dealsQuery,
  onTransition,
  onCardClick,
  mutationError,
  onDismissError,
}: PipelineBoardProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

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
            Could not load pipeline stages
            {stagesQuery.error ? `: ${stagesQuery.error.message}` : '.'}
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
          <span>Could not save move: {mutationError}</span>
          <button
            type="button"
            onClick={onDismissError}
            className="ml-4 underline hover:no-underline"
          >
            dismiss
          </button>
        </div>
      )}
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={handleDragEnd}
      >
        <div className="flex gap-4 overflow-x-auto pb-4" data-testid="pipeline-board">
          {stages.map((stage) => (
            <StageColumn
              key={stage.id}
              stage={stage}
              deals={dealsByStage.get(stage.id) ?? []}
              onCardClick={onCardClick}
            />
          ))}
        </div>
      </DndContext>
    </div>
  );
}

interface StageColumnProps {
  stage: PipelineStage;
  deals: Deal[];
  onCardClick: (dealId: string) => void;
}

function StageColumn({ stage, deals, onCardClick }: StageColumnProps) {
  const { isOver, setNodeRef } = useDroppable({ id: stage.id });

  return (
    <div
      ref={setNodeRef}
      data-testid={`pipeline-column-${stage.id}`}
      data-stage-name={stage.name}
      className={cn(
        'flex w-72 shrink-0 flex-col rounded-md border bg-muted/20 p-3',
        isOver && 'border-primary bg-primary/5',
      )}
    >
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold">{stage.name}</h2>
        <span className="text-xs text-muted-foreground">{deals.length}</span>
      </div>
      <div className="flex flex-col gap-2">
        {deals.length === 0 && (
          <p className="rounded-md border border-dashed py-6 text-center text-xs text-muted-foreground">
            No deals
          </p>
        )}
        {deals.map((deal) => (
          <DealCard key={deal.id} deal={deal} onClick={() => onCardClick(deal.id)} />
        ))}
      </div>
    </div>
  );
}

interface DealCardProps {
  deal: Deal;
  onClick: () => void;
}

function formatCurrency(amount: number | null, currency: string | null) {
  if (amount === null || !currency) return null;
  try {
    return new Intl.NumberFormat(undefined, {
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

function DealCard({ deal, onClick }: DealCardProps) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: deal.id,
  });
  const style: React.CSSProperties = {
    transform: CSS.Translate.toString(transform),
    opacity: isDragging ? 0.4 : 1,
  };
  const overdue = isOverdue(deal);
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
        'cursor-grab rounded-md border bg-background p-3 text-left shadow-sm transition-shadow hover:shadow-md',
        overdue && 'border-destructive/40',
      )}
    >
      <p className="line-clamp-2 text-sm font-medium">{deal.title}</p>
      <div className="mt-2 flex items-center justify-between text-xs text-muted-foreground">
        <span>{amount ?? '—'}</span>
        {deal.expected_close_date && (
          <span className={cn(overdue && 'text-destructive font-medium')}>
            {overdue ? 'Overdue ' : ''}
            {deal.expected_close_date}
          </span>
        )}
      </div>
    </div>
  );
}
