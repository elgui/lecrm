import type { Deal } from '@/lib/types';

export interface DealStats {
  /** Deals still in the pipeline (closed_at === null), across all currencies. */
  openCount: number;
  /**
   * Sum of `amount` for open deals **in `currency` only** — amounts are never
   * summed across currencies, so the headline value and its currency are always
   * coherent (a mixed USD+EUR pipeline reports the dominant currency's total,
   * not a meaningless cross-currency sum).
   */
  openValue: number;
  /**
   * Currency `openValue` is denominated in: the most frequent currency among
   * open deals that carry an amount. Defaults to 'EUR' (the demo/seed default)
   * when no open deal carries an amount.
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
  // Tally count + sum per currency so we never add amounts across currencies.
  const byCurrency = new Map<string, { count: number; sum: number }>();

  for (const deal of deals) {
    if (deal.closed_at !== null) continue;
    openCount += 1;
    if (deal.amount !== null) {
      const currency = deal.currency ?? 'EUR';
      const entry = byCurrency.get(currency) ?? { count: 0, sum: 0 };
      entry.count += 1;
      entry.sum += deal.amount;
      byCurrency.set(currency, entry);
    }
  }

  // Display the dominant currency (most open deals carrying an amount) and the
  // sum of ONLY that currency's deals. Defaults to EUR when no open deal carries
  // an amount. Map iteration is insertion-ordered, so ties keep the first-seen.
  let currency = 'EUR';
  let openValue = 0;
  let max = 0;
  for (const [cur, { count, sum }] of byCurrency) {
    if (count > max) {
      max = count;
      currency = cur;
      openValue = sum;
    }
  }

  return { openCount, openValue, currency };
}
