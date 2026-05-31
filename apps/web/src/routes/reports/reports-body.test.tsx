import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server.node';

import { ReportsBody, ReportsComingSoon } from './$workspaceId';
import { BASELINE_DASHBOARDS } from '@/lib/reports';
import { ApiError } from '@/lib/api';
import type { EmbedToken } from '@/hooks/use-embed-token';
import type { Deal, PaginatedResponse } from '@/lib/types';

// Minimal stand-ins for the React Query result shape. ReportsBody only
// reads .data, .error, .isLoading — keep the fakes tight.
type TokenQuery = {
  data: EmbedToken | undefined;
  error: Error | null;
  isLoading: boolean;
};
type DealsQuery = {
  data: PaginatedResponse<Deal> | undefined;
  error: Error | null;
  isLoading: boolean;
};

function fakeDealsLoaded(count: number): DealsQuery {
  const data: PaginatedResponse<Deal> = {
    data: Array.from({ length: count }).map((_, i) => ({
      id: `deal-${i}`,
      title: `Deal ${i}`,
      amount: null,
      currency: null,
      stage_id: null,
      contact_id: null,
      company_id: null,
      owner_id: null,
      expected_close_date: null,
      closed_at: null,
      created_at: '2026-05-01T00:00:00Z',
      updated_at: '2026-05-01T00:00:00Z',
    })),
    next_cursor: null,
    has_more: false,
  };
  return { data, error: null, isLoading: false };
}

const FIRST_DASH = BASELINE_DASHBOARDS[0]!;
const noop = () => {};

describe('<ReportsBody />', () => {
  it('renders the loading skeleton while the embed token is fetching', () => {
    const tokenQuery: TokenQuery = { data: undefined, error: null, isLoading: true };
    const dealsQuery = fakeDealsLoaded(3);
    const markup = renderToStaticMarkup(
      <ReportsBody
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        tokenQuery={tokenQuery as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery as any}
        active={FIRST_DASH}
        setActiveId={noop}
      />,
    );
    expect(markup).toContain('data-testid="reports-loading"');
    expect(markup).not.toContain('<iframe');
  });

  it('renders the empty-state card when the workspace has zero deals', () => {
    const tokenQuery: TokenQuery = {
      data: { token: 't', expires_at: '2026-05-28T13:05:00Z' },
      error: null,
      isLoading: false,
    };
    const dealsQuery = fakeDealsLoaded(0);
    const markup = renderToStaticMarkup(
      <ReportsBody
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        tokenQuery={tokenQuery as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery as any}
        active={FIRST_DASH}
        setActiveId={noop}
      />,
    );
    expect(markup).toContain('No data to report yet');
    expect(markup).not.toContain('<iframe');
  });

  it('renders the error card when the embed-token call fails with 403', () => {
    const tokenQuery: TokenQuery = {
      data: undefined,
      error: new ApiError(403, 'workspace mismatch'),
      isLoading: false,
    };
    const dealsQuery = fakeDealsLoaded(5);
    const markup = renderToStaticMarkup(
      <ReportsBody
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        tokenQuery={tokenQuery as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery as any}
        active={FIRST_DASH}
        setActiveId={noop}
      />,
    );
    expect(markup).toContain('Reports unavailable');
    expect(markup).toContain('do not have permission');
    expect(markup).not.toContain('<iframe');
  });

  it('renders the error card with a helpful message when embed reporting is disabled (503)', () => {
    const tokenQuery: TokenQuery = {
      data: undefined,
      error: new ApiError(503, 'embed reporting disabled'),
      isLoading: false,
    };
    const dealsQuery = fakeDealsLoaded(5);
    const markup = renderToStaticMarkup(
      <ReportsBody
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        tokenQuery={tokenQuery as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery as any}
        active={FIRST_DASH}
        setActiveId={noop}
      />,
    );
    expect(markup).toContain('not configured');
  });

  it('mounts the iframe with the active dashboard when token and data are present', () => {
    const tokenQuery: TokenQuery = {
      data: { token: 'live.jwt.token', expires_at: '2026-05-28T13:05:00Z' },
      error: null,
      isLoading: false,
    };
    const dealsQuery = fakeDealsLoaded(2);
    const markup = renderToStaticMarkup(
      <ReportsBody
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        tokenQuery={tokenQuery as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery as any}
        active={FIRST_DASH}
        setActiveId={noop}
      />,
    );
    expect(markup).toContain('<iframe');
    expect(markup).toContain('token=live.jwt.token');
    expect(markup).toContain(FIRST_DASH.title);
  });

  it('renders all four dashboards as tabs and marks the active one', () => {
    const tokenQuery: TokenQuery = {
      data: { token: 't', expires_at: '2026-05-28T13:05:00Z' },
      error: null,
      isLoading: false,
    };
    const dealsQuery = fakeDealsLoaded(1);
    const markup = renderToStaticMarkup(
      <ReportsBody
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        tokenQuery={tokenQuery as any}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        dealsQuery={dealsQuery as any}
        active={BASELINE_DASHBOARDS[2]!}
        setActiveId={noop}
      />,
    );
    for (const d of BASELINE_DASHBOARDS) {
      expect(markup).toContain(d.title);
    }
    // The third dashboard is the active one — tab should reflect that.
    expect(markup).toMatch(
      new RegExp(`aria-selected="true"[^>]*>${BASELINE_DASHBOARDS[2]!.title}`),
    );
  });
});

describe('<ReportsComingSoon />', () => {
  it('shows an honest branded placeholder — never the "not configured" error or an iframe', () => {
    const markup = renderToStaticMarkup(<ReportsComingSoon />);
    // The whole point of this task: the demo must never show the red
    // "Reports unavailable / not configured" error or a broken iframe.
    expect(markup).not.toContain('not configured');
    expect(markup).not.toContain('unavailable');
    expect(markup).not.toContain('<iframe');
    // Honest, on-brand framing (localized to French for the demo).
    expect(markup).toContain('Bientôt disponible');
    expect(markup).toContain('Rapports');
  });

  it('previews every baseline dashboard so it reads as a roadmap, not a dead end', () => {
    const markup = renderToStaticMarkup(<ReportsComingSoon />);
    for (const d of BASELINE_DASHBOARDS) {
      expect(markup).toContain(d.title);
      expect(markup).toContain(d.description);
    }
  });
});
