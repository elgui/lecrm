import { createRootRoute, Outlet, Link } from '@tanstack/react-router';
import { Users, Building2, CircleDollarSign, BarChart3, Kanban, Settings, LogOut } from 'lucide-react';
import { useAuth } from '@/hooks/use-auth';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';

const NAV_ITEMS = [
  { to: '/contacts' as const, label: 'Contacts', icon: Users },
  { to: '/companies' as const, label: 'Companies', icon: Building2 },
  { to: '/deals' as const, label: 'Deals', icon: CircleDollarSign },
  { to: '/settings' as const, label: 'Settings', icon: Settings },
];

function RootLayout() {
  const { user, isLoading, isUnauthenticated } = useAuth();

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="space-y-3">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-4 w-32" />
        </div>
      </div>
    );
  }

  if (isUnauthenticated) {
    window.location.href = '/auth/login';
    return null;
  }

  return (
    <div className="flex h-screen">
      <aside className="flex w-64 flex-col border-r bg-muted/30">
        <div className="flex h-14 items-center border-b px-4">
          <Link to="/" className="text-lg font-semibold">
            leCRM
          </Link>
        </div>

        <nav className="flex-1 space-y-1 p-2">
          {NAV_ITEMS.map(({ to, label, icon: Icon }) => (
            <Link
              key={to}
              to={to}
              className="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground [&.active]:bg-accent [&.active]:text-accent-foreground"
              activeProps={{ className: 'active' }}
            >
              <Icon className="h-4 w-4" />
              {label}
            </Link>
          ))}
          {user?.workspace_id && (
            <>
              <Link
                to="/pipeline/$workspaceId"
                params={{ workspaceId: user.workspace_id }}
                className="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground [&.active]:bg-accent [&.active]:text-accent-foreground"
                activeProps={{ className: 'active' }}
              >
                <Kanban className="h-4 w-4" />
                Pipeline
              </Link>
              <Link
                to="/reports/$workspaceId"
                params={{ workspaceId: user.workspace_id }}
                className="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground [&.active]:bg-accent [&.active]:text-accent-foreground"
                activeProps={{ className: 'active' }}
              >
                <BarChart3 className="h-4 w-4" />
                Reports
              </Link>
            </>
          )}
        </nav>

        <div className="border-t p-4">
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-xs text-primary-foreground">
              {user?.name?.charAt(0).toUpperCase() ?? '?'}
            </div>
            <div className="flex-1 truncate">
              <p className="truncate text-sm font-medium">{user?.name}</p>
              <p className="truncate text-xs text-muted-foreground">
                {user?.email}
              </p>
            </div>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={() => {
                window.location.href = '/auth/logout';
              }}
              title="Sign out"
            >
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </aside>

      <main className="flex-1 overflow-auto">
        <Outlet />
      </main>
    </div>
  );
}

export const Route = createRootRoute({
  component: RootLayout,
});
