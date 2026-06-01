import * as React from 'react';
import { Save } from 'lucide-react';

import {
  METRICS,
  PERIODS,
  dimensionOptions,
  type ReportDefinition,
  type ReportMetric,
  type ReportPeriod,
} from '@/lib/report-builder';
import type { PropertyDefinition } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

const selectClass =
  'h-10 w-full rounded-md border border-input bg-card px-3 text-sm shadow-xs focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/25';

export function ReportBuilderForm({
  definition,
  onChange,
  definitions,
  onSave,
  saving,
  savedId,
}: {
  definition: ReportDefinition;
  onChange: (next: ReportDefinition) => void;
  definitions: PropertyDefinition[] | undefined;
  onSave: () => void;
  saving: boolean;
  savedId: string | null;
}) {
  const dims = dimensionOptions(definitions);
  const yoyDisabled = definition.period === 'all';

  // When the period is set back to "all", N-1 cannot apply — clear it so the
  // backend never rejects the run (compare_yoy requires a bounded period).
  function setPeriod(period: ReportPeriod) {
    onChange({
      ...definition,
      period,
      compare_yoy: period === 'all' ? false : definition.compare_yoy,
    });
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Field label="Indicateur">
          <select
            className={selectClass}
            value={definition.metric}
            aria-label="Indicateur"
            onChange={(e) =>
              onChange({ ...definition, metric: e.target.value as ReportMetric })
            }
          >
            {METRICS.map((m) => (
              <option key={m.id} value={m.id}>
                {m.label}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Regroupement">
          <select
            className={selectClass}
            value={definition.dimension}
            aria-label="Regroupement"
            onChange={(e) => onChange({ ...definition, dimension: e.target.value })}
          >
            {dims.map((d) => (
              <option key={d.id} value={d.id}>
                {d.label}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Période">
          <select
            className={selectClass}
            value={definition.period}
            aria-label="Période"
            onChange={(e) => setPeriod(e.target.value as ReportPeriod)}
          >
            {PERIODS.map((p) => (
              <option key={p.id} value={p.id}>
                {p.label}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Comparaison">
          <label
            className={`flex h-10 items-center gap-2 rounded-md border border-input bg-card px-3 text-sm ${
              yoyDisabled ? 'opacity-50' : 'cursor-pointer'
            }`}
            title={
              yoyDisabled
                ? 'Choisissez une période (mois, trimestre ou année) pour comparer à N-1.'
                : 'Comparer à la même période un an plus tôt'
            }
          >
            <input
              type="checkbox"
              className="h-4 w-4 accent-primary"
              checked={definition.compare_yoy}
              disabled={yoyDisabled}
              aria-label="Comparer à N-1"
              onChange={(e) =>
                onChange({ ...definition, compare_yoy: e.target.checked })
              }
            />
            <span>Comparer à N-1</span>
          </label>
        </Field>
      </div>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
        <div className="flex-1">
          <Label htmlFor="report-name" className="mb-1.5 block text-xs text-muted-foreground">
            Nom du rapport (pour l’enregistrer)
          </Label>
          <Input
            id="report-name"
            placeholder="Ex. Analyse mensuelle — CA par étape"
            value={definition.name}
            onChange={(e) => onChange({ ...definition, name: e.target.value })}
          />
        </div>
        <Button
          type="button"
          variant="outline"
          disabled={saving || definition.name.trim() === ''}
          onClick={onSave}
          className="gap-2"
        >
          <Save className="h-4 w-4" />
          {savedId ? 'Mettre à jour' : 'Enregistrer'}
        </Button>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <span className="mb-1.5 block text-xs text-muted-foreground">{label}</span>
      {children}
    </div>
  );
}
