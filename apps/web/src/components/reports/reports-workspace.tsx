import * as React from 'react';
import { BarChart3, Sparkles, Trash2 } from 'lucide-react';

import {
  DEFAULT_DEFINITION,
  PRESET_REPORTS,
  definitionFrom,
  type ReportDefinition,
  type SavedReport,
} from '@/lib/report-builder';
import { useDealDefinitions } from '@/hooks/use-deals';
import {
  useRunReport,
  useSavedReports,
  useCreateSavedReport,
  useUpdateSavedReport,
  useDeleteSavedReport,
} from '@/hooks/use-reports';
import { ReportBuilderForm } from '@/components/reports/report-builder-form';
import { ReportResult } from '@/components/reports/report-result';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

// ReportsWorkspace is the live, native reporting surface: presets + a custom
// report builder (metric × dimension × period, N-1 toggle) wired to the
// /v1/reports/run + /v1/reports/definitions endpoints. It renders real
// workspace data on every deployment (no Cube container required).
export function ReportsWorkspace() {
  const [definition, setDefinition] = React.useState<ReportDefinition>(
    PRESET_REPORTS[0]!.definition,
  );
  // Tracks which saved report (if any) is currently loaded, so "Enregistrer"
  // updates it in place instead of creating a duplicate.
  const [savedId, setSavedId] = React.useState<string | null>(null);

  const definitions = useDealDefinitions();
  const runQuery = useRunReport(definition);
  const saved = useSavedReports();
  const createMut = useCreateSavedReport();
  const updateMut = useUpdateSavedReport();
  const deleteMut = useDeleteSavedReport();

  function loadDefinition(def: ReportDefinition, id: string | null) {
    setDefinition(def);
    setSavedId(id);
  }

  async function handleSave() {
    if (definition.name.trim() === '') return;
    if (savedId) {
      await updateMut.mutateAsync({ id: savedId, def: definition });
    } else {
      const created = await createMut.mutateAsync(definition);
      setSavedId(created.id);
    }
  }

  async function handleDelete(report: SavedReport) {
    await deleteMut.mutateAsync(report.id);
    if (savedId === report.id) {
      loadDefinition({ ...DEFAULT_DEFINITION, ...PRESET_REPORTS[0]!.definition }, null);
    }
  }

  const savedReports = saved.data ?? [];

  return (
    <div className="space-y-6">
      {/* Presets + saved reports: clicking one loads it into the builder. */}
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs font-medium text-muted-foreground">Rapports rapides :</span>
        {PRESET_REPORTS.map((p) => (
          <Chip
            key={p.id}
            active={savedId === null && sameShape(definition, p.definition)}
            onClick={() => loadDefinition({ ...p.definition }, null)}
          >
            {p.definition.name}
          </Chip>
        ))}
        {savedReports.map((r) => (
          <Chip
            key={r.id}
            active={savedId === r.id}
            onClick={() => loadDefinition(definitionFrom(r), r.id)}
          >
            <Sparkles className="mr-1 inline h-3 w-3" />
            {r.definition.name || 'Sans nom'}
          </Chip>
        ))}
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Constructeur de rapport</CardTitle>
        </CardHeader>
        <CardContent>
          <ReportBuilderForm
            definition={definition}
            onChange={setDefinition}
            definitions={definitions.data}
            onSave={handleSave}
            saving={createMut.isPending || updateMut.isPending}
            savedId={savedId}
          />
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-6">
          <ReportResult
            result={runQuery.data}
            isLoading={runQuery.isLoading}
            isError={runQuery.isError}
          />
        </CardContent>
      </Card>

      {savedReports.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Rapports enregistrés</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {savedReports.map((r) => (
              <div
                key={r.id}
                className="flex items-center justify-between rounded-md border border-border/60 px-3 py-2"
              >
                <button
                  className="flex items-center gap-2 text-left text-sm font-medium hover:text-primary"
                  onClick={() => loadDefinition(definitionFrom(r), r.id)}
                >
                  <BarChart3 className="h-4 w-4 text-muted-foreground" />
                  {r.definition.name || 'Sans nom'}
                  {r.definition.compare_yoy && (
                    <Badge variant="secondary" className="ml-1">N-1</Badge>
                  )}
                </button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={`Supprimer ${r.definition.name}`}
                  disabled={deleteMut.isPending}
                  onClick={() => handleDelete(r)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function Chip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'rounded-full border px-3 py-1 text-xs font-medium transition-colors',
        active
          ? 'border-primary bg-primary/10 text-primary'
          : 'border-border bg-card text-muted-foreground hover:text-foreground',
      )}
    >
      {children}
    </button>
  );
}

// sameShape compares everything except the name (presets carry a fixed name).
function sameShape(a: ReportDefinition, b: ReportDefinition): boolean {
  return (
    a.metric === b.metric &&
    a.dimension === b.dimension &&
    a.period === b.period &&
    a.compare_yoy === b.compare_yoy
  );
}
