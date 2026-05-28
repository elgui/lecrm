import * as React from 'react';
import {
  useTasks,
  useCreateTask,
  useToggleTask,
  useDeleteTask,
  type TaskScope,
} from '@/hooks/use-tasks';
import { useMe } from '@/hooks/use-me';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';

interface TasksPanelProps {
  /** When set, the panel lists & creates tasks scoped to one entity. */
  scope?: TaskScope;
  title?: string;
}

// TasksPanel lists tasks, creates them (title + due date), toggles
// completion, and deletes. Used both standalone (the /tasks route) and
// embedded on entity detail pages (scope set). Write controls gate on
// can_write — task mutations sit behind the admin+ RBAC guard.
export function TasksPanel({ scope, title = 'Tasks' }: TasksPanelProps) {
  const { permissions } = useMe();
  const canWrite = permissions.can_write;
  const { data: tasks, isLoading, error } = useTasks(scope);
  const create = useCreateTask();
  const toggle = useToggleTask();
  const remove = useDeleteTask();

  const [taskTitle, setTaskTitle] = React.useState('');
  const [dueDate, setDueDate] = React.useState('');

  const onCreate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!taskTitle.trim()) return;
    create.mutate(
      {
        title: taskTitle.trim(),
        due_date: dueDate || null,
        entity_type: scope?.entity_type ?? null,
        entity_id: scope?.entity_id ?? null,
      },
      {
        onSuccess: () => {
          setTaskTitle('');
          setDueDate('');
        },
      },
    );
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {canWrite && (
          <form onSubmit={onCreate} className="flex flex-wrap items-end gap-3">
            <div className="flex-1 space-y-2">
              <Label htmlFor="task-title">Title</Label>
              <Input
                id="task-title"
                value={taskTitle}
                onChange={(e) => setTaskTitle(e.target.value)}
                placeholder="Follow up…"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="task-due">Due date</Label>
              <Input
                id="task-due"
                type="date"
                value={dueDate}
                onChange={(e) => setDueDate(e.target.value)}
                className="w-40"
              />
            </div>
            <Button type="submit" disabled={create.isPending || !taskTitle.trim()}>
              {create.isPending ? 'Adding…' : 'Add task'}
            </Button>
          </form>
        )}
        {create.isError && (
          <p className="text-sm text-destructive">{(create.error as Error).message}</p>
        )}

        {isLoading && <Skeleton className="h-24 w-full" />}
        {error && (
          <p className="text-sm text-destructive">
            Failed to load tasks: {(error as Error).message}
          </p>
        )}

        {tasks && tasks.length === 0 && (
          <p className="text-sm text-muted-foreground">No tasks.</p>
        )}

        <ul className="divide-y">
          {tasks?.map((task) => {
            const done = !!task.completed_at;
            return (
              <li key={task.id} className="flex items-center gap-3 py-2">
                <input
                  type="checkbox"
                  checked={done}
                  disabled={!canWrite || toggle.isPending}
                  onChange={() => toggle.mutate(task.id)}
                  aria-label={`Mark ${task.title} ${done ? 'incomplete' : 'complete'}`}
                  className="h-4 w-4"
                />
                <div className="flex-1">
                  <p
                    className={
                      done
                        ? 'text-sm text-muted-foreground line-through'
                        : 'text-sm'
                    }
                  >
                    {task.title}
                  </p>
                  {task.due_date && (
                    <p className="text-xs text-muted-foreground">
                      Due {task.due_date}
                    </p>
                  )}
                </div>
                {canWrite && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={remove.isPending}
                    onClick={() => remove.mutate(task.id)}
                  >
                    Delete
                  </Button>
                )}
              </li>
            );
          })}
        </ul>
      </CardContent>
    </Card>
  );
}
