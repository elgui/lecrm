import { BarChart3 } from 'lucide-react';

import {
  delta,
  deltaTone,
  formatDelta,
  formatMetricValue,
  type RunResult,
} from '@/lib/report-builder';
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
  TableCell,
} from '@/components/ui/table';
import { EmptyState } from '@/components/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

const TONE_CLASS: Record<string, string> = {
  up: 'text-emerald-600 dark:text-emerald-400',
  down: 'text-destructive',
  flat: 'text-muted-foreground',
  new: 'text-primary',
};

export function ReportResult({
  result,
  isLoading,
  isError,
}: {
  result: RunResult | undefined;
  isLoading: boolean;
  isError: boolean;
}) {
  if (isLoading) {
    return (
      <div className="space-y-3" data-testid="report-loading">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (isError || !result) {
    return (
      <EmptyState
        icon={BarChart3}
        title="Impossible de charger ce rapport"
        description="Réessayez dans un instant. Si le problème persiste, vérifiez que des données existent pour la période choisie."
      />
    );
  }

  if (result.rows.length === 0) {
    return (
      <EmptyState
        icon={BarChart3}
        title="Aucune donnée pour ces critères"
        description="Aucune affaire ne correspond à la période et au regroupement sélectionnés."
      />
    );
  }

  const maxCurrent = Math.max(...result.rows.map((r) => Math.abs(r.current)), 1);

  return (
    <div className="space-y-3">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Regroupement</TableHead>
            <TableHead className="text-right">
              {result.compare_yoy ? result.current_label : result.metric_label}
            </TableHead>
            {result.compare_yoy && (
              <>
                <TableHead className="text-right">{result.prior_label}</TableHead>
                <TableHead className="text-right">Évolution</TableHead>
              </>
            )}
          </TableRow>
        </TableHeader>
        <TableBody>
          {result.rows.map((row, i) => {
            const prior = row.prior ?? 0;
            const d = result.compare_yoy ? delta(row.current, prior) : null;
            const tone = deltaTone(d);
            const widthPct = Math.round((Math.abs(row.current) / maxCurrent) * 100);
            return (
              <TableRow key={`${row.label}-${i}`}>
                <TableCell className="font-medium">
                  <div className="space-y-1">
                    <span>{row.label}</span>
                    {!result.compare_yoy && (
                      <div
                        className="h-1.5 rounded-full bg-primary/70"
                        style={{ width: `${Math.max(widthPct, 2)}%` }}
                        aria-hidden
                      />
                    )}
                  </div>
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatMetricValue(result.metric, row.current)}
                </TableCell>
                {result.compare_yoy && (
                  <>
                    <TableCell className="text-right tabular-nums text-muted-foreground">
                      {formatMetricValue(result.metric, prior)}
                    </TableCell>
                    <TableCell
                      className={cn('text-right tabular-nums font-medium', TONE_CLASS[tone])}
                    >
                      {formatDelta(d)}
                    </TableCell>
                  </>
                )}
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
      <p className="text-xs text-muted-foreground">
        {result.metric_label}
        {result.compare_yoy
          ? ` · ${result.current_label} vs ${result.prior_label} (N-1)`
          : ` · ${result.current_label}`}
      </p>
    </div>
  );
}
