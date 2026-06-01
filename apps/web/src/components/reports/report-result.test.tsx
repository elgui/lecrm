import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server.node';

import { ReportResult } from './report-result';
import type { RunResult } from '@/lib/report-builder';

const base: RunResult = {
  metric: 'deal_count',
  metric_label: "Nombre d'affaires",
  dimension: 'stage',
  period: 'all',
  compare_yoy: false,
  current_label: "Tout l'historique",
  rows: [
    { label: 'Découverte', current: 3 },
    { label: 'Qualifié', current: 1 },
  ],
  generated_at: '2026-06-01T00:00:00Z',
};

describe('<ReportResult />', () => {
  it('renders a loading skeleton', () => {
    const markup = renderToStaticMarkup(
      <ReportResult result={undefined} isLoading isError={false} />,
    );
    expect(markup).toContain('data-testid="report-loading"');
  });

  it('renders rows and the metric values without an N-1 column', () => {
    const markup = renderToStaticMarkup(
      <ReportResult result={base} isLoading={false} isError={false} />,
    );
    expect(markup).toContain('Découverte');
    expect(markup).toContain('Qualifié');
    expect(markup).not.toContain('Évolution');
  });

  it('renders the N-1 comparison column with the prior label and a delta', () => {
    const yoy: RunResult = {
      ...base,
      period: 'year',
      compare_yoy: true,
      current_label: '2026',
      prior_label: '2025',
      rows: [
        { label: 'Total', current: 10, prior: 8 }, // +25%
        { label: 'Nouveau', current: 5, prior: 0 }, // no baseline
      ],
    };
    const markup = renderToStaticMarkup(
      <ReportResult result={yoy} isLoading={false} isError={false} />,
    );
    expect(markup).toContain('2026');
    expect(markup).toContain('2025');
    expect(markup).toContain('Évolution');
    expect(markup).toContain('+25 %');
    expect(markup).toContain('nouveau');
  });

  it('renders an empty state when there are no rows', () => {
    const markup = renderToStaticMarkup(
      <ReportResult result={{ ...base, rows: [] }} isLoading={false} isError={false} />,
    );
    expect(markup).toContain('Aucune donnée');
  });

  it('renders an error state', () => {
    const markup = renderToStaticMarkup(
      <ReportResult result={undefined} isLoading={false} isError />,
    );
    expect(markup).toContain('Impossible de charger');
  });
});
