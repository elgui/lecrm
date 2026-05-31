import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface RecordSaveBarProps {
  canWrite: boolean;
  isDirty: boolean;
  isSaving: boolean;
  isSuccess: boolean;
  error?: string | null;
  onSave: () => void;
  className?: string;
}

// RecordSaveBar is the single save action for a record-detail page. One button
// persists both the core fields and the custom properties (the page wires both
// mutations into onSave), so there's no separate "Save properties" button to
// silently drop the other form's edits. Disabled until something is dirty.
export function RecordSaveBar({
  canWrite,
  isDirty,
  isSaving,
  isSuccess,
  error,
  onSave,
  className,
}: RecordSaveBarProps) {
  if (!canWrite) {
    return (
      <div
        className={cn(
          'rounded-lg border bg-card px-4 py-3 text-sm text-muted-foreground',
          className,
        )}
      >
        Accès en lecture seule. Demandez à un administrateur pour modifier.
      </div>
    );
  }

  return (
    <div
      className={cn(
        'flex items-center gap-3 rounded-lg border bg-card px-4 py-3',
        className,
      )}
    >
      <Button onClick={onSave} disabled={!isDirty || isSaving}>
        {isSaving ? 'Enregistrement…' : 'Enregistrer'}
      </Button>
      {isDirty && !isSaving && (
        <span className="text-sm text-muted-foreground">
          Modifications non enregistrées
        </span>
      )}
      {!isDirty && isSuccess && (
        <span className="text-sm font-medium text-emerald-600">Enregistré</span>
      )}
      {error && <span className="text-sm text-destructive">{error}</span>}
    </div>
  );
}
