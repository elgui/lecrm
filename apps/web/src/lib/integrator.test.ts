import { describe, expect, it } from 'vitest';
import {
  humanizeSlug,
  selectIntegratorContext,
} from './integrator';
import type { AccessibleWorkspace } from './types';

const ws = (slug: string, role: string): AccessibleWorkspace => ({
  slug,
  role,
  url: `https://${slug}.example.test`,
});

describe('humanizeSlug', () => {
  it('title-cases a hyphenated slug', () => {
    expect(humanizeSlug('bistrot-halles')).toBe('Bistrot Halles');
  });

  it('handles underscores and extra separators', () => {
    expect(humanizeSlug('menuiserie__vasseur')).toBe('Menuiserie Vasseur');
  });

  it('returns empty string for empty input', () => {
    expect(humanizeSlug('')).toBe('');
  });
});

describe('selectIntegratorContext', () => {
  it('flags an integrator administering the current workspace', () => {
    const workspaces = [
      ws('gb-consult', 'owner'),
      ws('bistrot-halles', 'integrator'),
    ];
    const ctx = selectIntegratorContext(workspaces, 'bistrot-halles');
    expect(ctx.isIntegrator).toBe(true);
    expect(ctx.clientSlug).toBe('bistrot-halles');
    expect(ctx.clientLabel).toBe('Bistrot Halles');
  });

  it('is not integrator when the current-workspace role is owner/admin/member', () => {
    const workspaces = [ws('bistrot-halles', 'owner')];
    expect(
      selectIntegratorContext(workspaces, 'bistrot-halles').isIntegrator,
    ).toBe(false);
  });

  it('is not integrator when the current slug is absent from the list', () => {
    const workspaces = [ws('bistrot-halles', 'integrator')];
    expect(
      selectIntegratorContext(workspaces, 'menuiserie-vasseur').isIntegrator,
    ).toBe(false);
  });

  it('is not integrator with an empty current slug', () => {
    const workspaces = [ws('bistrot-halles', 'integrator')];
    expect(selectIntegratorContext(workspaces, '').isIntegrator).toBe(false);
  });
});
