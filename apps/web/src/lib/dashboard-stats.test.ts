import { describe, expect, it } from 'vitest';

import { computeDealStats } from './dashboard-stats';
import type { Deal } from '@/lib/types';

function deal(partial: Partial<Deal>): Deal {
  return {
    id: 'd',
    title: 'Deal',
    amount: null,
    currency: null,
    stage_id: null,
    contact_id: null,
    company_id: null,
    owner_id: null,
    expected_close_date: null,
    closed_at: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...partial,
  };
}

describe('computeDealStats', () => {
  it('returns zeros for an empty pipeline', () => {
    expect(computeDealStats([])).toEqual({
      openCount: 0,
      openValue: 0,
      currency: 'EUR',
    });
  });

  it('counts only open deals and sums their amounts', () => {
    const stats = computeDealStats([
      deal({ amount: 8500, currency: 'EUR' }),
      deal({ amount: 14000, currency: 'EUR' }),
      // closed deals are excluded from both count and value
      deal({ amount: 6000, currency: 'EUR', closed_at: '2026-01-05T00:00:00Z' }),
    ]);
    expect(stats.openCount).toBe(2);
    expect(stats.openValue).toBe(22500);
  });

  it('ignores deals with no amount when summing value but still counts them', () => {
    const stats = computeDealStats([
      deal({ amount: 1000, currency: 'EUR' }),
      deal({ amount: null }),
    ]);
    expect(stats.openCount).toBe(2);
    expect(stats.openValue).toBe(1000);
  });

  it('picks the most frequent currency among open deals', () => {
    const stats = computeDealStats([
      deal({ amount: 100, currency: 'USD' }),
      deal({ amount: 200, currency: 'EUR' }),
      deal({ amount: 300, currency: 'EUR' }),
    ]);
    expect(stats.currency).toBe('EUR');
  });

  it('matches the seeded demo pipeline (4 open deals, 112_500 EUR)', () => {
    const seed = [
      deal({ amount: 8500, currency: 'EUR' }), // Discovery
      deal({ amount: 14000, currency: 'EUR' }), // Qualified
      deal({ amount: 52000, currency: 'EUR' }), // Proposal Sent
      deal({ amount: 38000, currency: 'EUR' }), // Negotiation
      deal({ amount: 6000, currency: 'EUR', closed_at: '2026-05-26T00:00:00Z' }), // won
      deal({ amount: 21000, currency: 'EUR', closed_at: '2026-05-19T00:00:00Z' }), // lost
    ];
    const stats = computeDealStats(seed);
    expect(stats).toEqual({
      openCount: 4,
      openValue: 112500,
      currency: 'EUR',
    });
  });
});
