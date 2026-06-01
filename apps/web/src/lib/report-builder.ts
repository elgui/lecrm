// Pure model + helpers for the native report builder. Mirrors the Go contract
// in apps/api/internal/reports/query.go: the wire `dimension` is either a bare
// kind ("none"|"stage"|"owner"|"company") or "custom:<property_key>"; metric /
// period are the same allow-listed ids. Keeping this side-effect-free makes the
// N-1 delta math, formatting, and serialization unit-testable without React.

import { formatAmount } from '@/lib/format';
import type { PropertyDefinition } from '@/lib/types';

export type ReportMetric = 'deal_count' | 'deal_amount_sum' | 'win_rate';
export type ReportPeriod = 'all' | 'month' | 'quarter' | 'year';

export interface ReportDefinition {
  name: string;
  metric: ReportMetric;
  dimension: string; // 'none' | 'stage' | 'owner' | 'company' | `custom:${key}`
  period: ReportPeriod;
  compare_yoy: boolean;
}

export interface RunRow {
  label: string;
  current: number;
  prior?: number | null;
}

export interface RunResult {
  metric: ReportMetric;
  metric_label: string;
  dimension: string;
  period: ReportPeriod;
  compare_yoy: boolean;
  current_label: string;
  prior_label?: string;
  rows: RunRow[];
  generated_at: string;
}

export interface SavedReport {
  id: string;
  definition: ReportDefinition;
  created_at: string;
  updated_at: string;
}

export const METRICS: { id: ReportMetric; label: string }[] = [
  { id: 'deal_count', label: "Nombre d'affaires" },
  { id: 'deal_amount_sum', label: 'Montant total (€)' },
  { id: 'win_rate', label: 'Taux de réussite' },
];

export const PERIODS: { id: ReportPeriod; label: string }[] = [
  { id: 'all', label: 'Tout l’historique' },
  { id: 'month', label: 'Ce mois-ci' },
  { id: 'quarter', label: 'Ce trimestre' },
  { id: 'year', label: 'Cette année' },
];

// Base (non-custom) dimensions.
export const BASE_DIMENSIONS: { id: string; label: string }[] = [
  { id: 'none', label: 'Total (aucun regroupement)' },
  { id: 'stage', label: 'Étape du pipeline' },
  { id: 'owner', label: 'Responsable' },
  { id: 'company', label: 'Société' },
];

export const DEFAULT_DEFINITION: ReportDefinition = {
  name: '',
  metric: 'deal_count',
  dimension: 'stage',
  period: 'all',
  compare_yoy: false,
};

// Preset reports shown before the user builds their own — they render live data
// immediately so the page is never an empty shell. Each maps to a native run.
export const PRESET_REPORTS: { id: string; definition: ReportDefinition }[] = [
  {
    id: 'deals-by-stage',
    definition: { name: 'Affaires par étape', metric: 'deal_count', dimension: 'stage', period: 'all', compare_yoy: false },
  },
  {
    id: 'amount-by-stage',
    definition: { name: 'Montant par étape', metric: 'deal_amount_sum', dimension: 'stage', period: 'all', compare_yoy: false },
  },
  {
    id: 'deals-by-company',
    definition: { name: 'Affaires par société', metric: 'deal_count', dimension: 'company', period: 'all', compare_yoy: false },
  },
];

export interface ParsedDimension {
  kind: 'none' | 'stage' | 'owner' | 'company' | 'custom';
  key: string; // property key when kind === 'custom'
}

export function parseDimension(dimension: string): ParsedDimension {
  if (dimension.startsWith('custom:')) {
    return { kind: 'custom', key: dimension.slice('custom:'.length) };
  }
  switch (dimension) {
    case 'stage':
    case 'owner':
    case 'company':
      return { kind: dimension, key: '' };
    default:
      return { kind: 'none', key: '' };
  }
}

// Build the full dimension option list, appending custom-property dimensions
// from the workspace's deal definitions.
export function dimensionOptions(
  definitions: PropertyDefinition[] | undefined,
): { id: string; label: string }[] {
  const custom = (definitions ?? []).map((d) => ({
    id: `custom:${d.property_key}`,
    label: `${d.display_name?.trim() || prettifyKey(d.property_key)} (champ perso)`,
  }));
  return [...BASE_DIMENSIONS, ...custom];
}

function prettifyKey(key: string): string {
  return key
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .trim();
}

/**
 * Year-over-year delta as a signed ratio (e.g. 0.25 = +25%).
 * - prior > 0  → (current - prior) / prior
 * - prior === 0 and current === 0 → 0 (no change)
 * - prior === 0 and current > 0 → null (no baseline; render as "nouveau")
 */
export function delta(current: number, prior: number): number | null {
  if (prior === 0) return current === 0 ? 0 : null;
  return (current - prior) / prior;
}

export type DeltaTone = 'up' | 'down' | 'flat' | 'new';

export function deltaTone(d: number | null): DeltaTone {
  if (d === null) return 'new';
  if (d > 0.0001) return 'up';
  if (d < -0.0001) return 'down';
  return 'flat';
}

export function formatDelta(d: number | null): string {
  if (d === null) return 'nouveau';
  const pct = d * 100;
  const sign = pct > 0 ? '+' : '';
  return `${sign}${pct.toFixed(pct % 1 === 0 ? 0 : 1)} %`;
}

// Format a metric value for display: currency for amounts, percent for win
// rate (stored 0..1), integer for counts.
export function formatMetricValue(metric: ReportMetric, value: number): string {
  switch (metric) {
    case 'deal_amount_sum':
      return formatAmount(value, 'EUR');
    case 'win_rate':
      return `${(value * 100).toFixed(value * 100 % 1 === 0 ? 0 : 1)} %`;
    default:
      return new Intl.NumberFormat('fr-FR').format(value);
  }
}

// Convert a SavedReport's definition into a fresh editable copy.
export function definitionFrom(saved: SavedReport): ReportDefinition {
  return { ...DEFAULT_DEFINITION, ...saved.definition };
}
