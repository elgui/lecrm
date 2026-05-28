import * as React from 'react';
import { useNotes, useCreateNote, useUpdateNote, useDeleteNote } from '@/hooks/use-notes';
import { useMe } from '@/hooks/use-me';
import type { EntityType } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';

interface NotesPanelProps {
  entityType: EntityType;
  entityId: string;
}

// NotesPanel renders the inline notes thread on an entity detail page:
// list, add, edit (own notes), delete (own notes). Write controls are gated
// on can_write — the API mounts note mutations behind the admin+ RBAC guard,
// so members see a read-only thread.
export function NotesPanel({ entityType, entityId }: NotesPanelProps) {
  const { me, permissions } = useMe();
  const canWrite = permissions.can_write;
  const { data: notes, isLoading, error } = useNotes(entityType, entityId);
  const create = useCreateNote(entityType, entityId);
  const update = useUpdateNote(entityType, entityId);
  const remove = useDeleteNote(entityType, entityId);

  const [draft, setDraft] = React.useState('');
  const [editingId, setEditingId] = React.useState<string | null>(null);
  const [editBody, setEditBody] = React.useState('');

  const authorId = me?.user_id ?? '';

  const onAdd = (e: React.FormEvent) => {
    e.preventDefault();
    if (!draft.trim() || !authorId) return;
    create.mutate(
      { body: draft.trim(), author_id: authorId },
      { onSuccess: () => setDraft('') },
    );
  };

  const onSaveEdit = (id: string) => {
    if (!editBody.trim()) return;
    update.mutate(
      { id, body: editBody.trim(), author_id: authorId },
      { onSuccess: () => setEditingId(null) },
    );
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">Notes</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {canWrite && (
          <form onSubmit={onAdd} className="space-y-2">
            <Textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder="Add a note…"
              aria-label="New note"
            />
            <Button type="submit" size="sm" disabled={create.isPending || !draft.trim()}>
              {create.isPending ? 'Adding…' : 'Add note'}
            </Button>
            {create.isError && (
              <p className="text-sm text-destructive">
                {(create.error as Error).message}
              </p>
            )}
          </form>
        )}

        {isLoading && <Skeleton className="h-20 w-full" />}
        {error && (
          <p className="text-sm text-destructive">
            Failed to load notes: {(error as Error).message}
          </p>
        )}

        {notes && notes.length === 0 && (
          <p className="text-sm text-muted-foreground">No notes yet.</p>
        )}

        <ul className="space-y-3">
          {notes?.map((note) => {
            const isOwn = note.author_id === authorId;
            const isEditing = editingId === note.id;
            return (
              <li key={note.id} className="rounded-md border p-3">
                {isEditing ? (
                  <div className="space-y-2">
                    <Textarea
                      value={editBody}
                      onChange={(e) => setEditBody(e.target.value)}
                      aria-label="Edit note"
                    />
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        onClick={() => onSaveEdit(note.id)}
                        disabled={update.isPending}
                      >
                        Save
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => setEditingId(null)}
                      >
                        Cancel
                      </Button>
                    </div>
                  </div>
                ) : (
                  <>
                    <p className="whitespace-pre-wrap text-sm">{note.body}</p>
                    <div className="mt-2 flex items-center justify-between">
                      <span className="text-xs text-muted-foreground">
                        {new Date(note.updated_at).toLocaleString()}
                      </span>
                      {canWrite && isOwn && (
                        <div className="flex gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => {
                              setEditingId(note.id);
                              setEditBody(note.body);
                            }}
                          >
                            Edit
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={remove.isPending}
                            onClick={() =>
                              remove.mutate({ id: note.id, author_id: authorId })
                            }
                          >
                            Delete
                          </Button>
                        </div>
                      )}
                    </div>
                  </>
                )}
              </li>
            );
          })}
        </ul>
      </CardContent>
    </Card>
  );
}
