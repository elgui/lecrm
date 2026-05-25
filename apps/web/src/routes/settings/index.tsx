import { createRoute } from '@tanstack/react-router';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { useAuth } from '@/hooks/use-auth';
import { Route as rootRoute } from '../__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: SettingsPage,
});

function SettingsPage() {
  const { user } = useAuth();

  return (
    <div className="p-8">
      <h1 className="mb-6 text-2xl font-semibold">Settings</h1>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Workspace</CardTitle>
          <CardDescription>
            Manage your workspace configuration
          </CardDescription>
        </CardHeader>
        <CardContent>
          <dl className="space-y-4">
            <div>
              <dt className="text-sm font-medium text-muted-foreground">
                Workspace ID
              </dt>
              <dd className="mt-1 text-sm">{user?.workspace_id ?? '-'}</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-muted-foreground">
                Slug
              </dt>
              <dd className="mt-1 text-sm">{user?.workspace_slug ?? '-'}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>
    </div>
  );
}
