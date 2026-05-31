import type { Deal } from '@/lib/types';

export interface DealStats {
  /** Deals still in the pipeline (closed_at === null). */
  openCount: number;
  /** Sum of `amount` for open deals that carry an amount. */
  openValue: number;
  /**
   * Currency to display `openValue` in: the most frequent currency among open
   * deals that have an amount. Defaults to 'EUR' (the demo/seed default) when
   * no open deal carries a currency.
   */
  currency: string;
}

/**
 * Aggregate the headline pipeline numbers shown on the dashboard from a list of
 * deals. A deal is "open" when it has not been closed (won or lost), i.e.
 * `closed_at` is null — mirroring the API model where `closed_at` is set only
 * once a deal leaves the active pipeline.
 *
 * Pure and side-effect free so it can be unit-tested and reused; the dashboard
 * feeds it the loaded page of deals (the demo's full set fits one page).
 */
export function computeDealStats(deals: Deal[]): DealStats {
  let openCount = 0;
  let openValue = 0;
  const currencyTally = new Map<string, number>();

  for (const deal of deals) {
    if (deal.closed_at !== null) continue;
    openCount += 1;
    if (deal.amount !== null) {
      openValue += deal.amount;
      const currency = deal.currency ?? 'EUR';
      currencyTally.set(currency, (currencyTally.get(currency) ?? 0) + 1);
    }
  }

  let currency = 'EUR';
  let max = 0;
  for (const [cur, n] of currencyTally) {
    if (n > max) {
      max = n;
      currency = cur;
    }
  }

  return { openCount, openValue, currency };
}
