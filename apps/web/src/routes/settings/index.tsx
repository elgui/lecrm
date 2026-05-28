import { createRoute } from '@tanstack/react-router';
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
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
    <div className="space-y-6 p-8">
      <h1 className="text-2xl font-semibold">Settings</h1>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Workspace</CardTitle>
          <CardDescription>Your workspace identity and configuration.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="ws-name">Workspace name</Label>
            <Input
              id="ws-name"
              defaultValue={user?.workspace_slug ?? ''}
              readOnly
              aria-readonly
            />
            <p className="text-xs text-muted-foreground">
              Renaming is provisioned by your integrator at v0; self-serve
              rename ships post-v0.
            </p>
          </div>
          <dl className="grid gap-4 sm:grid-cols-2">
            <div>
              <dt className="text-sm font-medium text-muted-foreground">Workspace ID</dt>
              <dd className="mt-1 text-sm">{user?.workspace_id ?? '-'}</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-muted-foreground">Slug</dt>
              <dd className="mt-1 text-sm">{user?.workspace_slug ?? '-'}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Branding</CardTitle>
          <CardDescription>
            Logo and accent colors for white-labeled workspaces.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Branding customization is a v0 placeholder — the AGPL source
            already exposes the theming hooks; the management UI lands post-v0.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
