import * as React from 'react';
import { X, GitMerge, Ban, ChevronRight, AlertCircle, CheckCircle2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import type { DedupContactPair, DedupCompanyPair } from '@/lib/types';
import {
  useContactDuplicates,
  useCompanyDuplicates,
  useMergeContacts,
  useMergeCompanies,
  useMarkContactsDistinct,
  useMarkCompaniesDistinct,
} from '@/hooks/use-dedup';

type Entity = 'contacts' | 'companies';

interface Props {
  entity: Entity;
  onClose: () => void;
}

// A generic pair with string fields so the component works for both entities.
interface GenericRecord {
  id: string;
  label: string; // display name
  fields: { key: string; label: string; aValue: string | null; bValue: string | null }[];
}

// Convert a contact pair to a generic representation.
function contactToGeneric(pair: DedupContactPair): GenericRecord & { aId: string; bId: string } {
  const { a, b } = pair;
  const aLabel = `${a.first_name} ${a.last_name}`.trim() || a.email || a.id;
  const bLabel = `${b.first_name} ${b.last_name}`.trim() || b.email || b.id;
  return {
    id: `${a.id}-${b.id}`,
    label: `${aLabel} / ${bLabel}`,
    aId: a.id,
    bId: b.id,
    fields: [
      { key: 'first_name', label: 'Prénom', aValue: a.first_name, bValue: b.first_name },
      { key: 'last_name', label: 'Nom', aValue: a.last_name, bValue: b.last_name },
      { key: 'email', label: 'E-mail', aValue: a.email, bValue: b.email },
      { key: 'phone', label: 'Téléphone', aValue: a.phone, bValue: b.phone },
      { key: 'company_id', label: 'Entreprise (ID)', aValue: a.company_id, bValue: b.company_id },
    ],
  };
}

function companyToGeneric(pair: DedupCompanyPair): GenericRecord & { aId: string; bId: string } {
  const { a, b } = pair;
  return {
    id: `${a.id}-${b.id}`,
    label: `${a.name} / ${b.name}`,
    aId: a.id,
    bId: b.id,
    fields: [
      { key: 'name', label: 'Nom', aValue: a.name, bValue: b.name },
      { key: 'domain', label: 'Domaine', aValue: a.domain, bValue: b.domain },
      { key: 'industry', label: 'Secteur', aValue: a.industry, bValue: b.industry },
    ],
  };
}

function reasonLabel(reason: string): { label: string; variant: 'default' | 'secondary' } {
  switch (reason) {
    case 'exact_email':
      return { label: 'E-mail identique', variant: 'default' };
    case 'exact_domain':
      return { label: 'Domaine identique', variant: 'default' };
    case 'similar_name':
      return { label: 'Nom similaire', variant: 'secondary' };
    default:
      return { label: reason, variant: 'secondary' };
  }
}

// FieldRow lets the user pick which side wins for a single field.
function FieldRow({
  label,
  fieldKey,
  aValue,
  bValue,
  survivorId,
  aId,
  resolver,
  onResolve,
}: {
  label: string;
  fieldKey: string;
  aValue: string | null;
  bValue: string | null;
  survivorId: string;
  aId: string;
  resolver: Record<string, 'survivor' | 'loser'>;
  onResolve: (key: string, side: 'survivor' | 'loser') => void;
}) {
  const conflicting = aValue !== bValue;
  const choice = resolver[fieldKey] ?? 'survivor';
  const survivorVal = survivorId === aId ? aValue : bValue;
  const loserVal = survivorId === aId ? bValue : aValue;
  const winnerVal = choice === 'survivor' ? survivorVal : loserVal;

  return (
    <div className="grid grid-cols-[120px_1fr_1fr_auto] items-center gap-2 border-b border-border py-2 last:border-0">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      {/* Survivor column */}
      <button
        type="button"
        onClick={() => conflicting && onResolve(fieldKey, 'survivor')}
        className={[
          'rounded px-2 py-1 text-left text-xs transition-colors',
          !conflicting ? 'cursor-default' : 'cursor-pointer hover:bg-accent',
          choice === 'survivor' ? 'bg-primary/10 ring-1 ring-primary' : '',
        ].join(' ')}
      >
        {survivorVal ?? <span className="italic text-muted-foreground">—</span>}
      </button>
      {/* Loser column */}
      <button
        type="button"
        onClick={() => conflicting && onResolve(fieldKey, 'loser')}
        className={[
          'rounded px-2 py-1 text-left text-xs transition-colors',
          !conflicting ? 'cursor-default' : 'cursor-pointer hover:bg-accent',
          choice === 'loser' ? 'bg-primary/10 ring-1 ring-primary' : '',
        ].join(' ')}
      >
        {loserVal ?? <span className="italic text-muted-foreground">—</span>}
      </button>
      {/* Winner indicator */}
      <span className="w-16 text-right text-xs text-muted-foreground">
        {conflicting ? (
          <span className={choice === 'survivor' ? 'text-primary' : 'text-muted-foreground'}>
            {winnerVal ?? '—'}
          </span>
        ) : (
          <CheckCircle2 className="ml-auto h-3.5 w-3.5 text-green-500" />
        )}
      </span>
    </div>
  );
}

// MergePanel shows field-by-field resolution for one pair.
function MergePanel({
  entity,
  aId,
  bId,
  fields,
  onDone,
  onBack,
}: {
  entity: Entity;
  aId: string;
  bId: string;
  fields: GenericRecord['fields'];
  onDone: () => void;
  onBack: () => void;
}) {
  const [survivorId, setSurvivorId] = React.useState(aId);
  const [resolver, setResolver] = React.useState<Record<string, 'survivor' | 'loser'>>({});
  const mergeContacts = useMergeContacts();
  const mergeCompanies = useMergeCompanies();
  const isPending = mergeContacts.isPending || mergeCompanies.isPending;
  const mergeError =
    (mergeContacts.error as Error | null) ?? (mergeCompanies.error as Error | null);

  const loserPanelId = survivorId === aId ? bId : aId;

  const onResolve = (key: string, side: 'survivor' | 'loser') => {
    setResolver((prev) => ({ ...prev, [key]: side }));
  };

  const handleMerge = () => {
    const payload = {
      survivor_id: survivorId,
      loser_id: loserPanelId,
      fields: resolver,
    };
    const mut = entity === 'contacts' ? mergeContacts : mergeCompanies;
    mut.mutate(payload as Parameters<typeof mut.mutate>[0], {
      onSuccess: onDone,
    });
  };

  return (
    <div className="flex flex-col gap-4">
      {/* Survivor selector */}
      <div className="rounded-lg border border-border bg-muted/40 p-3">
        <p className="mb-2 text-xs font-medium text-muted-foreground">Enregistrement survivant</p>
        <div className="flex gap-2">
          {[aId, bId].map((id) => (
            <button
              key={id}
              type="button"
              onClick={() => setSurvivorId(id)}
              className={[
                'flex-1 rounded-md border px-3 py-2 text-left text-xs transition-colors',
                survivorId === id
                  ? 'border-primary bg-primary/10 font-semibold'
                  : 'border-border hover:bg-accent',
              ].join(' ')}
            >
              <span className="block truncate font-mono">{id === aId ? 'A' : 'B'}</span>
              <span className="block truncate text-[10px] text-muted-foreground">{id}</span>
            </button>
          ))}
        </div>
      </div>

      {/* Field resolver */}
      <div className="rounded-lg border border-border bg-card p-3">
        <div className="mb-2 grid grid-cols-[120px_1fr_1fr_auto] gap-2 text-[10px] font-medium uppercase text-muted-foreground">
          <span>Champ</span>
          <span>Survivant ({survivorId === aId ? 'A' : 'B'})</span>
          <span>Perdant ({survivorId === aId ? 'B' : 'A'})</span>
          <span className="text-right">Résultat</span>
        </div>
        {fields.map((f) => (
          <FieldRow
            key={f.key}
            label={f.label}
            fieldKey={f.key}
            aValue={f.aValue}
            bValue={f.bValue}
            survivorId={survivorId}
            aId={aId}
            resolver={resolver}
            onResolve={onResolve}
          />
        ))}
      </div>

      <p className="text-xs text-muted-foreground">
        Notes, activités, tâches et deals liés au perdant seront re-associés au survivant.
      </p>

      {mergeError && (
        <div className="flex items-center gap-2 rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0" />
          {mergeError.message}
        </div>
      )}

      <div className="flex gap-2">
        <Button variant="outline" size="sm" onClick={onBack} disabled={isPending}>
          Retour
        </Button>
        <Button
          size="sm"
          onClick={handleMerge}
          disabled={isPending}
          className="ml-auto"
        >
          <GitMerge className="mr-1.5 h-4 w-4" />
          {isPending ? 'Fusion…' : 'Fusionner'}
        </Button>
      </div>
    </div>
  );
}

// PairListItem is a single row in the pair list.
function PairListItem({
  label,
  reason,
  score,
  aId,
  bId,
  entity,
  onMerge,
}: {
  label: string;
  reason: string;
  score: number;
  aId: string;
  bId: string;
  entity: Entity;
  onMerge: () => void;
}) {
  const markContactsDistinct = useMarkContactsDistinct();
  const markCompaniesDistinct = useMarkCompaniesDistinct();
  const { label: reasonText, variant } = reasonLabel(reason);
  const isMarkingDistinct =
    markContactsDistinct.isPending || markCompaniesDistinct.isPending;

  const handleDistinct = () => {
    const mut = entity === 'contacts' ? markContactsDistinct : markCompaniesDistinct;
    mut.mutate({ id_a: aId, id_b: bId });
  };

  return (
    <div className="flex items-center gap-3 border-b border-border px-4 py-3 last:border-0">
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{label}</p>
        <div className="mt-0.5 flex items-center gap-2">
          <Badge variant={variant} className="text-[10px]">
            {reasonText}
          </Badge>
          <span className="text-xs text-muted-foreground">{Math.round(score * 100)}%</span>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        <Button
          size="sm"
          variant="ghost"
          title="Marquer comme distincts (ne plus suggérer)"
          onClick={handleDistinct}
          disabled={isMarkingDistinct}
          className="h-8 px-2 text-muted-foreground hover:text-foreground"
        >
          <Ban className="h-4 w-4" />
        </Button>
        <Button size="sm" variant="outline" onClick={onMerge} className="h-8">
          <GitMerge className="mr-1 h-3.5 w-3.5" />
          Fusionner
          <ChevronRight className="ml-1 h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

export function DedupWizard({ entity, onClose }: Props) {
  const contactDups = useContactDuplicates();
  const companyDups = useCompanyDuplicates();

  const data = entity === 'contacts' ? contactDups : companyDups;
  const pairs =
    entity === 'contacts'
      ? (contactDups.data?.pairs ?? []).map(contactToGeneric)
      : (companyDups.data?.pairs ?? []).map(companyToGeneric);

  const [activePair, setActivePair] = React.useState<{
    aId: string;
    bId: string;
    fields: GenericRecord['fields'];
  } | null>(null);

  const handleMergeDone = () => {
    setActivePair(null);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-border bg-background shadow-xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border px-5 py-4">
          <div>
            <h2 className="text-base font-semibold">
              Doublons — {entity === 'contacts' ? 'Contacts' : 'Entreprises'}
            </h2>
            <p className="text-xs text-muted-foreground">
              {pairs.length} doublon{pairs.length !== 1 ? 's' : ''} détecté
              {pairs.length !== 1 ? 's' : ''}
            </p>
          </div>
          <Button variant="ghost" size="sm" onClick={onClose} className="h-8 w-8 p-0">
            <X className="h-4 w-4" />
          </Button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto">
          {data.isLoading && (
            <div className="flex flex-col gap-2 p-4">
              {[1, 2, 3].map((i) => (
                <div key={i} className="h-14 animate-pulse rounded bg-muted" />
              ))}
            </div>
          )}

          {data.isError && (
            <div className="flex items-center gap-2 p-4 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              Échec du chargement des doublons
            </div>
          )}

          {!data.isLoading && !data.isError && pairs.length === 0 && (
            <div className="flex flex-col items-center gap-2 py-12 text-center">
              <CheckCircle2 className="h-10 w-10 text-green-500" />
              <p className="text-sm font-medium">Aucun doublon détecté</p>
              <p className="text-xs text-muted-foreground">
                Votre base est propre ou les doublons ont été traités.
              </p>
            </div>
          )}

          {activePair ? (
            <div className="p-4">
              <MergePanel
                entity={entity}
                aId={activePair.aId}
                bId={activePair.bId}
                fields={activePair.fields}
                onDone={handleMergeDone}
                onBack={() => setActivePair(null)}
              />
            </div>
          ) : (
            pairs.map((p) => {
              const rawPair =
                entity === 'contacts'
                  ? contactDups.data?.pairs.find((cp) => `${cp.a.id}-${cp.b.id}` === p.id)
                  : companyDups.data?.pairs.find((cp) => `${cp.a.id}-${cp.b.id}` === p.id);
              const reason = rawPair?.reason ?? 'similar_name';
              const score = rawPair?.score ?? 0;
              return (
                <PairListItem
                  key={p.id}
                  label={p.label}
                  reason={reason}
                  score={score}
                  aId={p.aId}
                  bId={p.bId}
                  entity={entity}
                  onMerge={() =>
                    setActivePair({ aId: p.aId, bId: p.bId, fields: p.fields })
                  }
                />
              );
            })
          )}
        </div>

        {/* Footer */}
        {!activePair && (
          <div className="border-t border-border px-5 py-3">
            <Button variant="ghost" size="sm" onClick={onClose} className="ml-auto flex">
              Fermer
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
