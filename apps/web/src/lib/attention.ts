import type { Deal, Task } from '@/lib/types';

/** Milliseconds in one calendar day. */
const DAY_MS = 24 * 60 * 60 * 1000;

/** Local midnight for the given date, so diffs count calendar days not hours. */
function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

/**
 * Whole calendar days from `now` to `target` (negative when `target` is in the
 * past). Both ends are reduced to local midnight first, so a task due later
 * today reads as 0 days, not "−1" because of the clock time.
 */
export function daysBetween(now: Date, target: Date): number {
  return Math.round(
    (startOfDay(target).getTime() - startOfDay(now).getTime()) / DAY_MS,
  );
}

export interface AttentionTask {
  task: Task;
  /** Whole days until the due date; negative when overdue, null when undated. */
  daysUntilDue: number | null;
  overdue: boolean;
}

/**
 * Pick the open tasks that most deserve attention, soonest-due first.
 *
 * "Open" means not yet completed (`completed_at === null`). Dated tasks sort
 * ahead of undated ones (most overdue at the top); undated tasks fall to the
 * bottom, newest-created first so fresh work still surfaces. Pure and `now`-
 * injected so it is deterministic under test.
 */
export function selectAttentionTasks(
  tasks: Task[],
  now: Date,
  limit = 6,
): AttentionTask[] {
  const mapped: AttentionTask[] = tasks
    .filter((t) => t.completed_at === null)
    .map((task) => {
      if (!task.due_date) return { task, daysUntilDue: null, overdue: false };
      const due = new Date(task.due_date);
      if (Number.isNaN(due.getTime()))
        return { task, daysUntilDue: null, overdue: false };
      const days = daysBetween(now, due);
      return { task, daysUntilDue: days, overdue: days < 0 };
    });

  mapped.sort((a, b) => {
    if (a.daysUntilDue === null && b.daysUntilDue === null) {
      return b.task.created_at.localeCompare(a.task.created_at);
    }
    if (a.daysUntilDue === null) return 1;
    if (b.daysUntilDue === null) return -1;
    return a.daysUntilDue - b.daysUntilDue;
  });

  return mapped.slice(0, limit);
}

export interface ClosingDeal {
  deal: Deal;
  /** Whole days until the expected close; negative when the date has passed. */
  daysUntilClose: number;
  overdue: boolean;
}

/**
 * Pick the open deals whose expected close is within `withinDays` (default 14),
 * soonest first. Deals whose close date has already passed but are still open
 * are included (they need attention most) and flagged `overdue`. Closed deals
 * (`closed_at !== null`) and those with no expected close date are skipped.
 */
export function selectClosingDeals(
  deals: Deal[],
  now: Date,
  withinDays = 14,
  limit = 6,
): ClosingDeal[] {
  const closing: ClosingDeal[] = [];
  for (const deal of deals) {
    if (deal.closed_at !== null) continue;
    if (!deal.expected_close_date) continue;
    const close = new Date(deal.expected_close_date);
    if (Number.isNaN(close.getTime())) continue;
    const days = daysBetween(now, close);
    if (days > withinDays) continue;
    closing.push({ deal, daysUntilClose: days, overdue: days < 0 });
  }
  closing.sort((a, b) => a.daysUntilClose - b.daysUntilClose);
  return closing.slice(0, limit);
}

/**
 * Short French relative-day label for a calendar-day offset, e.g. −2 →
 * "En retard de 2 j", 0 → "Aujourd'hui", 1 → "Demain", 5 → "Dans 5 j".
 */
export function relativeDayLabel(days: number): string {
  if (days < -1) return `En retard de ${-days} j`;
  if (days === -1) return 'Hier';
  if (days === 0) return "Aujourd'hui";
  if (days === 1) return 'Demain';
  return `Dans ${days} j`;
}
