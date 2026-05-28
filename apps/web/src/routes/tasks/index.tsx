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
    <div className="p-8">
      <h1 className="mb-6 text-2xl font-semibold">Tasks</h1>
      <TasksPanel title="All tasks" />
    </div>
  );
}
