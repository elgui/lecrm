import * as React from 'react';
import { Upload, X, ChevronRight, ChevronLeft, Download, Check, AlertCircle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';

type Entity = 'contacts' | 'companies' | 'deals';

interface CoreField {
  key: string;
  label: string;
  required: boolean;
}

interface CustomField {
  key: string;
  label: string;
  property_type: string;
}

interface AnalyzeResp {
  columns: string[];
  sample_rows: string[][];
  row_count: number;
  core_fields: CoreField[];
  custom_fields: CustomField[];
  suggested_mapping: Record<string, string>;
}

interface RowOutcome {
  line: number;
  action: 'create' | 'update' | 'skip' | 'error';
  reason?: string;
  label?: string;
}

interface Summary {
  total: number;
  create: number;
  update: number;
  skip: number;
  error: number;
}

interface PreviewResp {
  summary: Summary;
  rows: RowOutcome[];
}

interface CommitResp {
  summary: Summary;
  error_report_csv: string;
  audit_event: string;
}

const DEDUPE_OPTIONS = [
  { value: 'update', label: 'Mettre à jour si correspondance' },
  { value: 'skip', label: 'Ignorer si correspondance' },
  { value: 'create', label: 'Créer toujours (doublons autorisés)' },
] as const;

type DedupePolicy = 'update' | 'skip' | 'create';

type Step = 'upload' | 'mapping' | 'preview' | 'done';

interface Props {
  entity: Entity;
  onClose: () => void;
}

export function CsvImportWizard({ entity, onClose }: Props) {
  const [step, setStep] = React.useState<Step>('upload');
  const [csvText, setCsvText] = React.useState('');
  const [fileName, setFileName] = React.useState('');
  const [analyze, setAnalyze] = React.useState<AnalyzeResp | null>(null);
  const [mapping, setMapping] = React.useState<Record<string, string>>({});
  const [dedupe, setDedupe] = React.useState<DedupePolicy>('update');
  const [preview, setPreview] = React.useState<PreviewResp | null>(null);
  const [commit, setCommit] = React.useState<CommitResp | null>(null);
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const fileRef = React.useRef<HTMLInputElement>(null);

  const entityLabel = entity === 'contacts' ? 'contacts' : entity === 'companies' ? 'entreprises' : 'affaires';

  // Build the target options for the column mapping dropdown.
  const targetOptions = React.useMemo(() => {
    if (!analyze) return [];
    const opts: { value: string; label: string }[] = [
      { value: '', label: '— Ignorer —' },
      ...analyze.core_fields.map((f) => ({
        value: f.key,
        label: `${f.label}${f.required ? ' *' : ''}`,
      })),
      ...(analyze.custom_fields ?? []).map((f) => ({
        value: `cf_${f.key}`,
        label: `${f.label} (personnalisé)`,
      })),
    ];
    return opts;
  }, [analyze]);

  async function callImport<T>(step: string, body: unknown): Promise<T> {
    const res = await fetch(`/v1/import/${entity}/${step}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const msg = await res.text().catch(() => 'Erreur inconnue');
      throw new Error(msg);
    }
    return res.json() as Promise<T>;
  }

  // Step 1: parse the chosen file.
  const handleFile = (file: File) => {
    setFileName(file.name);
    const reader = new FileReader();
    reader.onload = (e) => {
      const text = e.target?.result as string;
      setCsvText(text);
    };
    reader.readAsText(file, 'UTF-8');
  };

  const handleFileDrop = (e: React.DragEvent) => {
    e.preventDefault();
    const file = e.dataTransfer.files[0];
    if (file) handleFile(file);
  };

  // Step 2: call analyze, move to mapping step.
  const handleAnalyze = async () => {
    setError(null);
    setBusy(true);
    try {
      const result = await callImport<AnalyzeResp>('analyze', { csv: csvText });
      setAnalyze(result);
      setMapping(result.suggested_mapping ?? {});
      setStep('mapping');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Analyse échouée');
    } finally {
      setBusy(false);
    }
  };

  // Step 3: dry-run preview.
  const handlePreview = async () => {
    setError(null);
    setBusy(true);
    try {
      const result = await callImport<PreviewResp>('preview', {
        csv: csvText,
        mapping,
        dedupe,
      });
      setPreview(result);
      setStep('preview');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Aperçu échoué');
    } finally {
      setBusy(false);
    }
  };

  // Step 4: commit.
  const handleCommit = async () => {
    setError(null);
    setBusy(true);
    try {
      const result = await callImport<CommitResp>('commit', {
        csv: csvText,
        mapping,
        dedupe,
      });
      setCommit(result);
      setStep('done');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Import échoué');
    } finally {
      setBusy(false);
    }
  };

  // Download the error report.
  const downloadErrorReport = () => {
    if (!commit?.error_report_csv) return;
    const blob = new Blob([commit.error_report_csv], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${entity}_import_errors.csv`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  };

  return (
    // Full-screen backdrop
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="relative flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-border bg-background shadow-xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border px-6 py-4">
          <div>
            <h2 className="text-lg font-semibold">Importer des {entityLabel}</h2>
            <p className="mt-0.5 text-sm text-muted-foreground">
              {step === 'upload' && 'Étape 1 / 3 — Choisissez un fichier CSV'}
              {step === 'mapping' && 'Étape 2 / 3 — Associez les colonnes'}
              {step === 'preview' && 'Étape 3 / 3 — Aperçu avant import'}
              {step === 'done' && 'Import terminé'}
            </p>
          </div>
          <button
            onClick={onClose}
            className="rounded-md p-1 text-muted-foreground hover:text-foreground"
            aria-label="Fermer"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-6 py-5">
          {/* ── Step 1: Upload ── */}
          {step === 'upload' && (
            <div className="space-y-4">
              <div
                className="flex cursor-pointer flex-col items-center justify-center rounded-lg border-2 border-dashed border-border bg-muted/30 p-10 transition-colors hover:border-primary/50 hover:bg-muted/50"
                onClick={() => fileRef.current?.click()}
                onDrop={handleFileDrop}
                onDragOver={(e) => e.preventDefault()}
              >
                <Upload className="mb-3 h-8 w-8 text-muted-foreground" />
                <p className="text-sm font-medium text-foreground">
                  {fileName ? fileName : 'Glissez votre CSV ici ou cliquez pour choisir'}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  Format UTF-8, jusqu'à 16 Mo, première ligne = en-têtes
                </p>
                <input
                  ref={fileRef}
                  type="file"
                  accept=".csv,text/csv"
                  className="hidden"
                  onChange={(e) => {
                    const f = e.target.files?.[0];
                    if (f) handleFile(f);
                  }}
                />
              </div>
              {csvText && (
                <p className="text-sm text-muted-foreground">
                  Fichier chargé en mémoire ({Math.round(csvText.length / 1024)} Ko)
                </p>
              )}
            </div>
          )}

          {/* ── Step 2: Column mapping ── */}
          {step === 'mapping' && analyze && (
            <div className="space-y-5">
              <div className="rounded-md border border-border bg-muted/20 p-3 text-sm">
                <span className="font-medium">{analyze.row_count}</span> lignes de données détectées
                {analyze.sample_rows.length > 0 && (
                  <span className="ml-2 text-muted-foreground">
                    (aperçu : {analyze.sample_rows.slice(0, 2).map((r) => r[0]).join(', ')}…)
                  </span>
                )}
              </div>

              {/* Dedup policy (only for contacts/companies) */}
              {entity !== 'deals' && (
                <div className="space-y-2">
                  <Label>En cas de correspondance</Label>
                  <div className="flex flex-col gap-1.5">
                    {DEDUPE_OPTIONS.map((opt) => (
                      <label key={opt.value} className="flex cursor-pointer items-center gap-2 text-sm">
                        <input
                          type="radio"
                          name="dedupe"
                          value={opt.value}
                          checked={dedupe === opt.value}
                          onChange={() => setDedupe(opt.value as DedupePolicy)}
                          className="accent-primary"
                        />
                        {opt.label}
                      </label>
                    ))}
                  </div>
                </div>
              )}

              {/* Column mapping table */}
              <div className="space-y-2">
                <Label>Association des colonnes</Label>
                <div className="overflow-hidden rounded-lg border border-border">
                  <table className="w-full text-sm">
                    <thead className="bg-muted/50">
                      <tr>
                        <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                          Colonne CSV
                        </th>
                        <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                          Champ cible
                        </th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-border">
                      {analyze.columns.map((col) => (
                        <tr key={col} className="hover:bg-muted/20">
                          <td className="px-3 py-2 font-mono text-xs text-foreground">{col}</td>
                          <td className="px-3 py-2">
                            <select
                              value={mapping[col] ?? ''}
                              onChange={(e) =>
                                setMapping((m) => ({ ...m, [col]: e.target.value }))
                              }
                              className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-primary/50"
                            >
                              {targetOptions.map((opt) => (
                                <option key={opt.value} value={opt.value}>
                                  {opt.label}
                                </option>
                              ))}
                            </select>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          )}

          {/* ── Step 3: Preview ── */}
          {step === 'preview' && preview && (
            <div className="space-y-4">
              <SummaryBadges summary={preview.summary} />

              {preview.rows.length > 0 && (
                <div className="overflow-hidden rounded-lg border border-border">
                  <table className="w-full text-sm">
                    <thead className="bg-muted/50">
                      <tr>
                        <th className="w-12 px-3 py-2 text-left font-medium text-muted-foreground">
                          #
                        </th>
                        <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                          Libellé
                        </th>
                        <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                          Action
                        </th>
                        <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                          Raison
                        </th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-border">
                      {preview.rows.slice(0, 20).map((row) => (
                        <tr key={row.line} className="hover:bg-muted/20">
                          <td className="px-3 py-1.5 tabular-nums text-muted-foreground">
                            {row.line}
                          </td>
                          <td className="px-3 py-1.5 text-foreground">{row.label || '—'}</td>
                          <td className="px-3 py-1.5">
                            <ActionBadge action={row.action} />
                          </td>
                          <td className="px-3 py-1.5 text-muted-foreground">{row.reason || ''}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  {preview.rows.length > 20 && (
                    <p className="border-t border-border px-3 py-2 text-xs text-muted-foreground">
                      … et {preview.rows.length - 20} autres lignes
                    </p>
                  )}
                </div>
              )}
            </div>
          )}

          {/* ── Step 4: Done ── */}
          {step === 'done' && commit && (
            <div className="space-y-4">
              <div className="flex items-center gap-3 rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-800 dark:bg-green-950/30">
                <Check className="h-5 w-5 shrink-0 text-green-600 dark:text-green-400" />
                <p className="text-sm font-medium text-green-800 dark:text-green-300">
                  Import réussi
                </p>
              </div>
              <SummaryBadges summary={commit.summary} />
              {commit.error_report_csv && commit.summary.error > 0 && (
                <Button variant="outline" size="sm" onClick={downloadErrorReport}>
                  <Download className="mr-2 h-4 w-4" />
                  Télécharger le rapport d'erreurs ({commit.summary.error} ligne
                  {commit.summary.error > 1 ? 's' : ''})
                </Button>
              )}
            </div>
          )}

          {/* Inline error */}
          {error && (
            <div className="mt-4 flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between border-t border-border px-6 py-4">
          <div>
            {step === 'mapping' && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setStep('upload')}
                disabled={busy}
              >
                <ChevronLeft className="mr-1 h-4 w-4" />
                Retour
              </Button>
            )}
            {step === 'preview' && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setStep('mapping')}
                disabled={busy}
              >
                <ChevronLeft className="mr-1 h-4 w-4" />
                Retour
              </Button>
            )}
          </div>
          <div className="flex gap-2">
            {step === 'done' ? (
              <Button onClick={onClose}>Fermer</Button>
            ) : (
              <>
                <Button variant="outline" onClick={onClose} disabled={busy}>
                  Annuler
                </Button>
                {step === 'upload' && (
                  <Button onClick={handleAnalyze} disabled={!csvText || busy}>
                    {busy ? 'Analyse…' : 'Suivant'}
                    {!busy && <ChevronRight className="ml-1 h-4 w-4" />}
                  </Button>
                )}
                {step === 'mapping' && (
                  <Button onClick={handlePreview} disabled={busy}>
                    {busy ? 'Aperçu…' : 'Aperçu'}
                    {!busy && <ChevronRight className="ml-1 h-4 w-4" />}
                  </Button>
                )}
                {step === 'preview' && (
                  <Button onClick={handleCommit} disabled={busy}>
                    {busy ? 'Import en cours…' : 'Importer'}
                    {!busy && <ChevronRight className="ml-1 h-4 w-4" />}
                  </Button>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// ── Small sub-components ──

function SummaryBadges({ summary }: { summary: Summary }) {
  return (
    <div className="flex flex-wrap gap-2 text-sm">
      <span className="rounded-full bg-muted px-3 py-1 font-medium">
        {summary.total} total
      </span>
      {summary.create > 0 && (
        <span className="rounded-full bg-green-100 px-3 py-1 font-medium text-green-800 dark:bg-green-900/30 dark:text-green-300">
          {summary.create} créations
        </span>
      )}
      {summary.update > 0 && (
        <span className="rounded-full bg-blue-100 px-3 py-1 font-medium text-blue-800 dark:bg-blue-900/30 dark:text-blue-300">
          {summary.update} mises à jour
        </span>
      )}
      {summary.skip > 0 && (
        <span className="rounded-full bg-yellow-100 px-3 py-1 font-medium text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300">
          {summary.skip} ignorées
        </span>
      )}
      {summary.error > 0 && (
        <span className="rounded-full bg-red-100 px-3 py-1 font-medium text-red-800 dark:bg-red-900/30 dark:text-red-300">
          {summary.error} erreurs
        </span>
      )}
    </div>
  );
}

function ActionBadge({ action }: { action: RowOutcome['action'] }) {
  const map = {
    create: 'rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-800 dark:bg-green-900/30 dark:text-green-300',
    update: 'rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
    skip: 'rounded-full bg-yellow-100 px-2 py-0.5 text-xs font-medium text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300',
    error: 'rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-800 dark:bg-red-900/30 dark:text-red-300',
  };
  const label = { create: 'créer', update: 'mettre à jour', skip: 'ignorer', error: 'erreur' };
  return <span className={map[action]}>{label[action]}</span>;
}
