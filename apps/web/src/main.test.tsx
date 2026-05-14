import { describe, expect, it } from 'vitest';

import { cn } from '@/lib/utils';

describe('cn utility', () => {
  it('merges class names', () => {
    expect(cn('a', 'b')).toBe('a b');
  });

  it('drops falsy values', () => {
    expect(cn('a', false && 'b', undefined, null, 'c')).toBe('a c');
  });

  it('lets tailwind-merge resolve conflicts', () => {
    expect(cn('p-2', 'p-4')).toBe('p-4');
  });
});
