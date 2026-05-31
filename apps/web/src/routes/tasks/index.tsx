import { createRoute } from '@tanstack/react-router';
import { TasksPanel } from '@/components/tasks-panel';
import { Route as rootRoute } from '../__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/tasks',
  component: TasksPage,
});

// Global, workspace-wide task list. Entity-scoped task panels live on the
// contact/company/deal detail pages; this is the unscoped view.
function TasksPage() {
  return (
    <div className="mx-auto max-w-4xl p-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight">Tâches</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Tous vos suivis, regroupés au même endroit.
        </p>
      </div>
      <TasksPanel title="Toutes les tâches" />
    </div>
  );
}
