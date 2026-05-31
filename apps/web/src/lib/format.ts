import type { BadgeProps } from '@/components/ui/badge';

type BadgeVariant = NonNullable<BadgeProps['variant']>;

/**
 * Map a free-form pipeline stage name to a semantic badge color.
 * Falls back to a neutral pill for unknown stages.
 */
export function stageBadgeVariant(stage: string): BadgeVariant {
  const s = stage.toLowerCase();
  if (/(won|closed won|complete|success)/.test(s)) return 'success';
  if (/(lost|closed lost|cancel|churn|dead)/.test(s)) return 'destructive';
  if (/(proposal|negotiat|contract|review)/.test(s)) return 'warning';
  if (/(qualified|demo|meeting|engaged)/.test(s)) return 'default';
  return 'secondary';
}

/**
 * Format a monetary amount that may arrive as a number or string.
 * Returns an em dash for empty values and the raw value if unparseable.
 */
export function formatAmount(
  amount: number | string | null | undefined,
  currency = 'USD',
): string {
  if (amount === null || amount === undefined || amount === '') return '—';
  const n = typeof amount === 'number' ? amount : Number(amount);
  if (Number.isNaN(n)) return String(amount);
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency,
    maximumFractionDigits: n % 1 === 0 ? 0 : 2,
  }).format(n);
}
