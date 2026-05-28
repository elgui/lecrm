// Baseline dashboard catalogue rendered inside the Cube.dev iframe at
// /reports/$workspaceId. Each entry maps to a Cube query against the
// schemas in deploy/cube/schema/ (Deals.js, Activities.js,
// PipelineStages.js).
//
// The frontend renders these by passing the dashboard id in the iframe
// URL hash; the Cube-side embed page picks up the id and runs the
// matching query through the Cube REST API with the JWT we mint.
//
// Keep the catalogue here (single source of truth on the frontend)
// rather than hardcoding URLs — the iframe URL builder reads it.

export type CubeQuery = {
  measures?: string[];
  dimensions?: string[];
  timeDimensions?: Array<{
    dimension: string;
    granularity?: 'day' | 'week' | 'month';
    dateRange?: string | [string, string];
  }>;
  order?: Record<string, 'asc' | 'desc'>;
  limit?: number;
};

export interface DashboardSpec {
  id: string;
  title: string;
  description: string;
  // chartType is a hint for the embed page; falls back to bar.
  chartType: 'bar' | 'line' | 'funnel' | 'table';
  query: CubeQuery;
}

export const BASELINE_DASHBOARDS: DashboardSpec[] = [
  {
    id: 'deals-by-stage',
    title: 'Deals by stage',
    description: 'Open deals grouped by pipeline stage.',
    chartType: 'bar',
    query: {
      measures: ['Deals.count'],
      dimensions: ['Deals.dealStage'],
      order: { 'Deals.count': 'desc' },
    },
  },
  {
    id: 'deals-by-owner',
    title: 'Deals by owner',
    description: 'Top 10 owners by deal count and total value.',
    chartType: 'table',
    query: {
      measures: ['Deals.count', 'Deals.totalAmount'],
      dimensions: ['Deals.ownerId'],
      order: { 'Deals.totalAmount': 'desc' },
      limit: 10,
    },
  },
  {
    id: 'recent-activities',
    title: 'Recent activities',
    description: 'Activities created in the last 30 days, by type.',
    chartType: 'line',
    query: {
      measures: ['Activities.count'],
      dimensions: ['Activities.activityType'],
      timeDimensions: [
        {
          dimension: 'Activities.createdAt',
          granularity: 'day',
          dateRange: 'last 30 days',
        },
      ],
    },
  },
  {
    id: 'conversion-funnel',
    title: 'Conversion funnel',
    description: 'Deal counts progressing through pipeline stages.',
    chartType: 'funnel',
    query: {
      measures: ['Deals.count'],
      dimensions: ['Deals.dealStage'],
      order: { 'Deals.dealStage': 'asc' },
    },
  },
];

export function findDashboard(id: string): DashboardSpec | undefined {
  return BASELINE_DASHBOARDS.find((d) => d.id === id);
}

// Resolves the base URL of the embedded Cube dashboard frontend.
// In production this is fronted by Caddy at /cube/embed; in dev it
// can be overridden via VITE_CUBE_EMBED_URL pointed at a local Cube
// playground or static embed page.
export function cubeEmbedBaseUrl(): string {
  const env = (import.meta as ImportMeta & { env?: Record<string, string | undefined> }).env;
  return env?.VITE_CUBE_EMBED_URL ?? '/cube/embed';
}

// Builds the iframe src for a given token + dashboard.
// We pass the JWT in the query string (the embed page reads it
// directly and forwards it as the `Authorization: Bearer` header to
// the Cube API). The dashboard id sits in the hash so it never leaks
// to the Cube backend logs.
export function buildCubeFrameUrl(
  token: string,
  dashboardId: string,
  baseUrl: string = cubeEmbedBaseUrl(),
): string {
  // SSR / test-server contexts (renderToStaticMarkup, vitest jsdom)
  // may lack `window`; the URL parser still needs an absolute base.
  const origin =
    typeof window !== 'undefined' && window.location?.origin
      ? window.location.origin
      : 'http://localhost';
  const url = new URL(baseUrl, origin);
  url.searchParams.set('token', token);
  url.hash = `dashboard=${encodeURIComponent(dashboardId)}`;
  return url.toString();
}
