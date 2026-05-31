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
    title: 'Affaires par étape',
    description: 'Affaires en cours regroupées par étape du pipeline.',
    chartType: 'bar',
    query: {
      measures: ['Deals.count'],
      dimensions: ['Deals.dealStage'],
      order: { 'Deals.count': 'desc' },
    },
  },
  {
    id: 'deals-by-owner',
    title: 'Affaires par responsable',
    description: 'Top 10 des responsables par nombre d’affaires et valeur totale.',
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
    title: 'Activités récentes',
    description: 'Activités créées sur les 30 derniers jours, par type.',
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
    title: 'Entonnoir de conversion',
    description: 'Progression du nombre d’affaires à travers les étapes du pipeline.',
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

// Whether embedded reporting (Cube.dev) is wired up in this deployment.
// Reporting needs a running Cube container, the per-workspace RO roles,
// the `/cube/embed` chart frontend, and LECRM_CUBE_JWT_SECRET on the
// API — none of which are provisioned on the public demo. Until that
// stack exists, the Reports route renders an honest "coming soon"
// placeholder instead of letting the embed-token call 503 into a red
// "not configured" error during the demo. Flip VITE_REPORTS_ENABLED to
// "true" once Cube is actually deployed to light up the live path.
// The `flag` param defaults to the build-time env value; tests pass an
// explicit value (Vite statically inlines import.meta.env, so it can't
// be stubbed at runtime — same reason cubeEmbedBaseUrl takes an arg).
export function reportsEnabled(
  flag: string | undefined = (
    import.meta as ImportMeta & { env?: Record<string, string | undefined> }
  ).env?.VITE_REPORTS_ENABLED,
): boolean {
  return flag === 'true';
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
