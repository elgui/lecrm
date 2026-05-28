import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server.node';

import { CubeFrame } from './cube-frame';
import { BASELINE_DASHBOARDS } from '@/lib/reports';

describe('CubeFrame', () => {
  it('renders an iframe with the embed URL containing the token and dashboard id', () => {
    const dashboard = BASELINE_DASHBOARDS[0]!;
    const markup = renderToStaticMarkup(
      <CubeFrame token="server.signed.jwt" dashboard={dashboard} />,
    );
    expect(markup).toContain('<iframe');
    expect(markup).toContain('token=server.signed.jwt');
    expect(markup).toContain(`#dashboard=${dashboard.id}`);
    // Resize listener is wired up; default min height should appear in
    // the inline style so the frame has space even before the first
    // postMessage resize event.
    expect(markup).toMatch(/height:\s*480px/);
  });

  it('uses a sandbox attribute that allows scripts but not same-origin', () => {
    const markup = renderToStaticMarkup(
      <CubeFrame token="t" dashboard={BASELINE_DASHBOARDS[0]!} />,
    );
    expect(markup).toMatch(/sandbox="allow-scripts allow-popups"/);
    expect(markup).not.toContain('allow-same-origin');
  });

  it('renders an accessible title that includes the dashboard name', () => {
    const dashboard = BASELINE_DASHBOARDS[1]!;
    const markup = renderToStaticMarkup(
      <CubeFrame token="t" dashboard={dashboard} />,
    );
    expect(markup).toContain(`Cube dashboard — ${dashboard.title}`);
  });
});
