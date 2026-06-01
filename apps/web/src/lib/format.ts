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
 * French-market product → format with the fr-FR locale (space thousands
 * separator, trailing currency symbol).
 */
export function formatAmount(
  amount: number | string | null | undefined,
  currency = 'EUR',
): string {
  if (amount === null || amount === undefined || amount === '') return '—';
  const n = typeof amount === 'number' ? amount : Number(amount);
  if (Number.isNaN(n)) return String(amount);
  return new Intl.NumberFormat('fr-FR', {
    style: 'currency',
    currency,
    maximumFractionDigits: n % 1 === 0 ? 0 : 2,
  }).format(n);
}

// Coerce a stored value into a Date. Date-only strings ("YYYY-MM-DD", as the
// API returns for expected_close_date / due_date) are pinned to local midnight
// so they never shift a day under a negative-offset timezone.
function toDate(value: string | number | Date): Date {
  if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return new Date(value + 'T00:00:00');
  }
  return new Date(value);
}

/**
 * Format a date as the French/EU `JJ/MM/AAAA` (e.g. `30/05/2026`).
 * Returns an em dash for empty values and the raw value if unparseable.
 */
export function formatDate(
  value: string | number | Date | null | undefined,
): string {
  if (value === null || value === undefined || value === '') return '—';
  const d = toDate(value);
  if (Number.isNaN(d.getTime())) return String(value);
  return new Intl.DateTimeFormat('fr-FR', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
  }).format(d);
}

/**
 * Format a date+time in the French locale (`30/05/2026 14:05`).
 * Returns an em dash for empty values and the raw value if unparseable.
 */
export function formatDateTime(
  value: string | number | Date | null | undefined,
): string {
  if (value === null || value === undefined || value === '') return '—';
  const d = toDate(value);
  if (Number.isNaN(d.getTime())) return String(value);
  return new Intl.DateTimeFormat('fr-FR', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(d);
}

/**
 * Relative French date for human-friendly, touch-first surfaces (mobile cards,
 * deal due-dates): `aujourd'hui`, `demain`, `hier`, `dans 14 jours`,
 * `il y a 3 jours`. Beyond ±13 days it falls back to the compact absolute date
 * (`13 juil.`) so distant dates stay legible. The reference "now" is injectable
 * for deterministic tests. Returns an empty string for empty values.
 */
export function formatDateRelative(
  value: string | number | Date | null | undefined,
  now: Date = new Date(),
): string {
  if (value === null || value === undefined || value === '') return '';
  const d = toDate(value);
  if (Number.isNaN(d.getTime())) return String(value);

  // Diff in whole calendar days, both pinned to local midnight so a date-only
  // value never lands a few hours off and reads as "demain" when it's today.
  const startOfDay = (x: Date) =>
    new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diffDays = Math.round((startOfDay(d) - startOfDay(now)) / 86_400_000);

  if (diffDays === 0) return "aujourd'hui";
  if (diffDays === 1) return 'demain';
  if (diffDays === -1) return 'hier';
  if (diffDays > 1 && diffDays <= 13) return `dans ${diffDays} jours`;
  if (diffDays < -1 && diffDays >= -13) return `il y a ${-diffDays} jours`;
  return formatDateShort(d);
}

/**
 * Compact French date for dense surfaces like pipeline cards (`13 juil.`).
 * Returns an empty string for empty values so callers can omit the element.
 */
export function formatDateShort(
  value: string | number | Date | null | undefined,
): string {
  if (value === null || value === undefined || value === '') return '';
  const d = toDate(value);
  if (Number.isNaN(d.getTime())) return String(value);
  return new Intl.DateTimeFormat('fr-FR', {
    day: 'numeric',
    month: 'short',
  }).format(d);
}
