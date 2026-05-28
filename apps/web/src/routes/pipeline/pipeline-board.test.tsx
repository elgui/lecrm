import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server.node';

import { PipelineBoard } from './$workspaceId';
import type { PipelineStage } from '@/hooks/use-pipeline-stages';
import type { Deal, PaginatedResponse } from '@/lib/types';

type StagesQuery = {
  data: { data: PipelineStage[] } | undefined;
  error: Error | null;
  isLoading: boolean;
};
type DealsQuery = {
  data: PaginatedResponse<Deal> | undefined;
  error: Error | null;
  isLoading: boolean;
};

const STAGES: PipelineStage[] = [
  { id: 's1', name: 'Discovery', order_index: 1, created_at: '2026-05-28T00:00:00Z' },
  { id: 's2', name: 'Qualified', order_index: 2, created_at: '2026-05-28T00:00:00Z' },
  { id: 's3', name: 'Proposal Sent', order_index: 3, created_at: '2026-05-28T00:00:00Z' },
  { id: 's4', name: 'Negotiation', order_index: 4, created_at: '2026-05-28T00:00:00Z' },
  { id: 's5', name: 'Closed-Won/Lost', order_index: 5, created_at: '2026-05-28T00:00:00Z' },
];

function makeDeal(id: string, stageId: string | null, title = `Deal ${id}`): Deal {
  return {
    id,
    title,
    amount: 1000,
    currency: 'USD',
    stage_id: stageId,
    contact_id: null,
    company_id: null,
    owner_id: null,
    expected_close_date: null,
    closed_at: null,
    created_at: '2026-05-28T00:00:00Z',
    updated_at: '2026-05-28T00:00:00Z',
  };
}

function stagesQuery(data: PipelineStage[] | undefined, opts: Partial<StagesQuery> = {}): StagesQuery {
  return {
    data: data ? { data } : undefined,
    error: opts.error ?? null,
    isLoading: opts.isLoading ?? false,
  };
}
function dealsQuery(data: Deal[] | undefined, opts: Partial<DealsQuery> = {}): DealsQuery {
  return {
    data: data
      ? { data, next_cursor: null, has_more: false }
      : undefined,
    error: opts.error ?? null,
    isLoading: opts.isLoading ?? false,
  };
}

const noop = () => {};

describe('<PipelineBoard />', () => {
  it('renders one column per stage in seeded order', () => {
    const deals = [
      makeDeal('d1', 's1'),
      makeDeal('d2', 's2'),
      makeDeal('d3', 's2'),
    ];
    const markup = renderToStaticMarkup(
      <PipelineBoard
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        stagesQuery={stagesQuery(STAGES) as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery(deals) as any}
        onTransition={noop}
        onCardClick={noop}
        mutationError={null}
        onDismissError={noop}
      />,
    );
    for (const s of STAGES) {
      expect(markup).toContain(`data-stage-name="${s.name}"`);
    }
    // Order: Discovery's column appears before Closed-Won/Lost's column.
    const idxDisc = markup.indexOf('data-stage-name="Discovery"');
    const idxClosed = markup.indexOf('data-stage-name="Closed-Won/Lost"');
    expect(idxDisc).toBeGreaterThan(-1);
    expect(idxClosed).toBeGreaterThan(idxDisc);
  });

  it('places each deal in the column matching its stage_id', () => {
    const deals = [
      makeDeal('d1', 's1', 'Alpha'),
      makeDeal('d2', 's3', 'Bravo'),
    ];
    const markup = renderToStaticMarkup(
      <PipelineBoard
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        stagesQuery={stagesQuery(STAGES) as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery(deals) as any}
        onTransition={noop}
        onCardClick={noop}
        mutationError={null}
        onDismissError={noop}
      />,
    );
    // Both cards rendered
    expect(markup).toContain('Alpha');
    expect(markup).toContain('Bravo');
    // Find the substring belonging to each column and assert which titles it contains.
    const discoveryStart = markup.indexOf('data-testid="pipeline-column-s1"');
    const proposalStart = markup.indexOf('data-testid="pipeline-column-s3"');
    const qualifiedStart = markup.indexOf('data-testid="pipeline-column-s2"');
    expect(discoveryStart).toBeGreaterThan(-1);
    expect(proposalStart).toBeGreaterThan(-1);
    expect(qualifiedStart).toBeGreaterThan(-1);
    // Card "Alpha" should appear in the slice belonging to the Discovery column.
    const discoverySlice = markup.slice(discoveryStart, qualifiedStart);
    expect(discoverySlice).toContain('Alpha');
    expect(discoverySlice).not.toContain('Bravo');
  });

  it('shows the empty-state placeholder in columns without deals', () => {
    const markup = renderToStaticMarkup(
      <PipelineBoard
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        stagesQuery={stagesQuery(STAGES) as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery([]) as any}
        onTransition={noop}
        onCardClick={noop}
        mutationError={null}
        onDismissError={noop}
      />,
    );
    // All five columns should show "No deals" because the deal list is empty.
    const matches = markup.match(/No deals/g) ?? [];
    expect(matches.length).toBe(STAGES.length);
  });

  it('renders a loading placeholder while data is being fetched', () => {
    const markup = renderToStaticMarkup(
      <PipelineBoard
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        stagesQuery={stagesQuery(undefined, { isLoading: true }) as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery(undefined, { isLoading: true }) as any}
        onTransition={noop}
        onCardClick={noop}
        mutationError={null}
        onDismissError={noop}
      />,
    );
    expect(markup).toContain('data-testid="pipeline-loading"');
  });

  it('renders an error card if the stages query failed', () => {
    const markup = renderToStaticMarkup(
      <PipelineBoard
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        stagesQuery={stagesQuery(undefined, { error: new Error('boom') }) as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery([]) as any}
        onTransition={noop}
        onCardClick={noop}
        mutationError={null}
        onDismissError={noop}
      />,
    );
    expect(markup).toContain('Could not load pipeline stages');
    expect(markup).toContain('boom');
  });

  it('renders the inline error banner when a transition fails', () => {
    const markup = renderToStaticMarkup(
      <PipelineBoard
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        stagesQuery={stagesQuery(STAGES) as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery([makeDeal('d1', 's1')]) as any}
        onTransition={noop}
        onCardClick={noop}
        mutationError="server rejected the move"
        onDismissError={vi.fn()}
      />,
    );
    expect(markup).toContain('server rejected the move');
    expect(markup).toContain('role="alert"');
  });
});
