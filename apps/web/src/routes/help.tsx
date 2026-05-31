import { createRoute } from '@tanstack/react-router';
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { useMe } from '@/hooks/use-me';
import { Route as rootRoute } from './__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/help',
  component: HelpPage,
});

function HelpPage() {
  const { role } = useMe();

  return (
    <div className="mx-auto max-w-4xl space-y-6 p-8">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Help</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          How leCRM works, who can do what, and how to get support.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Quick start</CardTitle>
          <CardDescription>Get productive in a few minutes.</CardDescription>
        </CardHeader>
        <CardContent>
          <ol className="list-decimal space-y-2 pl-5 text-sm text-muted-foreground">
            <li>
              Add a <strong>Contact</strong> or <strong>Company</strong> from
              the left-hand navigation.
            </li>
            <li>
              Create a <strong>Deal</strong> and track its value and stage.
            </li>
            <li>
              Open the <strong>Pipeline</strong> (Kanban) to drag deals through
              your sales stages.
            </li>
            <li>
              Log follow-ups in <strong>Tasks</strong> so nothing slips.
            </li>
            <li>
              Use <strong>Reports</strong> for an at-a-glance overview, and{' '}
              <strong>Settings → Custom Fields</strong> to tailor records to
              your business.
            </li>
          </ol>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Accounts &amp; access</CardTitle>
          <CardDescription>
            Each person has a role scoped to this workspace.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm text-muted-foreground">
          {role !== 'none' && (
            <p>
              Your current role: <Badge variant="secondary">{role}</Badge>
            </p>
          )}
          <ul className="space-y-2">
            <li>
              <strong>Member</strong> — read-only access to all records.
            </li>
            <li>
              <strong>Admin</strong> — everything a member can do, plus create
              and edit records and manage custom fields.
            </li>
            <li>
              <strong>Owner</strong> — everything an admin can do, plus invite
              or remove members and change their roles.
            </li>
          </ul>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Workspaces &amp; client accounts</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm text-muted-foreground">
          <p>
            Each client is a separate workspace on its own address (for example{' '}
            <code>client.lecrm.gbconsult.me</code>). Your data, members, and
            settings are isolated per workspace.
          </p>
          <p>
            To work in a different client&apos;s data, sign in to that client&apos;s
            address. Switching between client accounts from inside the app is on
            the roadmap and not available yet.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Need a hand?</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          <p>
            Contact your leCRM administrator for access changes, new workspaces,
            or anything that isn&apos;t working as expected.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
