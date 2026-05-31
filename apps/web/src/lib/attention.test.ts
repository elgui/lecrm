import { describe, it, expect } from 'vitest';
import {
  daysBetween,
  selectAttentionTasks,
  selectClosingDeals,
  relativeDayLabel,
} from './attention';
import type { Deal, Task } from '@/lib/types';

const NOW = new Date('2026-05-31T10:00:00Z');

function task(overrides: Partial<Task>): Task {
  return {
    id: 'task-' + (overrides.id ?? Math.random().toString(36).slice(2)),
    title: 'Task',
    description: null,
    entity_type: null,
    entity_id: null,
    assignee_id: null,
    due_date: null,
    completed_at: null,
    created_at: '2026-05-01T00:00:00Z',
    updated_at: '2026-05-01T00:00:00Z',
    ...overrides,
  };
}

function deal(overrides: Partial<Deal>): Deal {
  return {
    id: 'deal-' + (overrides.id ?? Math.random().toString(36).slice(2)),
    title: 'Deal',
    amount: 1000,
    currency: 'EUR',
    stage_id: 'stage-1',
    contact_id: null,
    company_id: null,
    owner_id: null,
    expected_close_date: null,
    closed_at: null,
    created_at: '2026-05-01T00:00:00Z',
    updated_at: '2026-05-01T00:00:00Z',
    ...overrides,
  };
}

describe('daysBetween', () => {
  it('counts calendar days, ignoring time of day', () => {
    // Same calendar day, later clock time → 0, not −1.
    expect(daysBetween(NOW, new Date('2026-05-31T23:00:00Z'))).toBe(0);
    expect(daysBetween(NOW, new Date('2026-06-02T01:00:00Z'))).toBe(2);
    expect(daysBetween(NOW, new Date('2026-05-29T23:00:00Z'))).toBe(-2);
  });
});

describe('selectAttentionTasks', () => {
  it('excludes completed tasks', () => {
    const result = selectAttentionTasks(
      [
        task({ id: 'done', completed_at: '2026-05-30T00:00:00Z', due_date: '2026-06-01T00:00:00Z' }),
        task({ id: 'open', due_date: '2026-06-01T00:00:00Z' }),
      ],
      NOW,
    );
    expect(result.map((r) => r.task.id)).toEqual(['task-open']);
  });

  it('flags overdue tasks and sorts most-overdue first', () => {
    const result = selectAttentionTasks(
      [
        task({ id: 'soon', due_date: '2026-06-03T00:00:00Z' }),
        task({ id: 'overdue', due_date: '2026-05-28T00:00:00Z' }),
        task({ id: 'today', due_date: '2026-05-31T18:00:00Z' }),
      ],
      NOW,
    );
    expect(result.map((r) => r.task.id)).toEqual([
      'task-overdue',
      'task-today',
      'task-soon',
    ]);
    expect(result[0].overdue).toBe(true);
    expect(result[0].daysUntilDue).toBe(-3);
    expect(result[1].overdue).toBe(false);
    expect(result[1].daysUntilDue).toBe(0);
  });

  it('sorts undated tasks last, newest-created first', () => {
    const result = selectAttentionTasks(
      [
        task({ id: 'undated-old', created_at: '2026-05-01T00:00:00Z' }),
        task({ id: 'dated', due_date: '2026-06-10T00:00:00Z' }),
        task({ id: 'undated-new', created_at: '2026-05-20T00:00:00Z' }),
      ],
      NOW,
    );
    expect(result.map((r) => r.task.id)).toEqual([
      'task-dated',
      'task-undated-new',
      'task-undated-old',
    ]);
  });

  it('respects the limit', () => {
    const tasks = Array.from({ length: 10 }, (_, i) =>
      task({ id: String(i), due_date: `2026-06-${String(i + 1).padStart(2, '0')}T00:00:00Z` }),
    );
    expect(selectAttentionTasks(tasks, NOW, 3)).toHaveLength(3);
  });
});

describe('selectClosingDeals', () => {
  it('includes only open deals closing within the window', () => {
    const result = selectClosingDeals(
      [
        deal({ id: 'soon', expected_close_date: '2026-06-05T00:00:00Z' }),
        deal({ id: 'far', expected_close_date: '2026-07-30T00:00:00Z' }),
        deal({ id: 'no-date', expected_close_date: null }),
        deal({
          id: 'closed',
          expected_close_date: '2026-06-02T00:00:00Z',
          closed_at: '2026-05-20T00:00:00Z',
        }),
      ],
      NOW,
    );
    expect(result.map((r) => r.deal.id)).toEqual(['deal-soon']);
  });

  it('includes still-open past-due deals and flags them overdue, soonest first', () => {
    const result = selectClosingDeals(
      [
        deal({ id: 'tomorrow', expected_close_date: '2026-06-01T00:00:00Z' }),
        deal({ id: 'late', expected_close_date: '2026-05-25T00:00:00Z' }),
      ],
      NOW,
    );
    expect(result.map((r) => r.deal.id)).toEqual(['deal-late', 'deal-tomorrow']);
    expect(result[0].overdue).toBe(true);
    expect(result[0].daysUntilClose).toBe(-6);
    expect(result[1].overdue).toBe(false);
    expect(result[1].daysUntilClose).toBe(1);
  });
});

describe('relativeDayLabel', () => {
  it('renders French relative-day labels', () => {
    expect(relativeDayLabel(-2)).toBe('En retard de 2 j');
    expect(relativeDayLabel(-1)).toBe('Hier');
    expect(relativeDayLabel(0)).toBe("Aujourd'hui");
    expect(relativeDayLabel(1)).toBe('Demain');
    expect(relativeDayLabel(5)).toBe('Dans 5 j');
  });
});
