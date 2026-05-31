import type { BadgeProps } from '@/components/ui/badge';

type BadgeVariant = NonNullable<BadgeProps['variant']>;

/**
 * Map a free-form pipeline stage name to a semantic badge color.
 * Falls back to a neutral pill for unknown stages.
 */
export function stageBadgeVariant(stage: string): BadgeVariant {
  const s = stage.toLowerCase();
  // Patterns cover both the English gbconsult-default labels and the French
  // labels used by the demo workspace (accented + unaccented forms, since the
  // accent can be stripped depending on the data source). A combined
  // "Gagné / Perdu" stage hits the won branch first → success, matching the
  // legacy "Closed-Won/Lost" behaviour.
  if (/(won|closed won|complete|success|gagné|gagne)/.test(s)) return 'success';
  if (/(lost|closed lost|cancel|churn|dead|perdu)/.test(s)) return 'destructive';
  if (/(proposal|negotiat|contract|review|proposition|négocia|negocia)/.test(s))
    return 'warning';
  if (/(qualified|demo|meeting|engaged|qualifié|qualifie|découverte|decouverte)/.test(s))
    return 'default';
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
