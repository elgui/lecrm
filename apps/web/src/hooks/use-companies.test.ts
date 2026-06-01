import { describe, it, expect } from 'vitest';
import { companyNameMap } from './use-companies';
import type { Company } from '@/lib/types';

function company(id: string, name: string): Company {
  return {
    id,
    name,
    domain: null,
    industry: null,
    size: null,
    owner_id: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  };
}

describe('companyNameMap', () => {
  it('resolves a company id to its name', () => {
    const map = companyNameMap([
      company('c0000000-0000-0000-0000-000000000001', 'Bistrot des Halles'),
      company('c0000000-0000-0000-0000-000000000002', 'Menuiserie Vasseur'),
    ]);
    expect(map.get('c0000000-0000-0000-0000-000000000001')).toBe('Bistrot des Halles');
    expect(map.get('c0000000-0000-0000-0000-000000000002')).toBe('Menuiserie Vasseur');
  });

  it('returns undefined for an unknown id (caller renders a clean dash, never a UUID)', () => {
    const map = companyNameMap([company('c1', 'Acme')]);
    expect(map.get('does-not-exist')).toBeUndefined();
  });

  it('handles an empty company list', () => {
    expect(companyNameMap([]).size).toBe(0);
  });
});
