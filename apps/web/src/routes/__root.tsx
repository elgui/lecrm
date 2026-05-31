import { createRootRoute, Outlet, Link } from '@tanstack/react-router';
import type { LinkProps } from '@tanstack/react-router';
import {
  Users,
  Building2,
  CircleDollarSign,
  BarChart3,
  Kanban,
  CheckSquare,
  Settings,
  UserCog,
  SlidersHorizontal,
  LogOut,
  CircleHelp,
  type LucideIcon,
} from 'lucide-react';
import { useAuth } from '@/hooks/use-auth';
import { useMe } from '@/hooks/use-me';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { WorkspaceSwitcher } from '@/components/WorkspaceSwitcher';

const navLinkClass =
  'group relative flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-sidebar-foreground transition-colors hover:bg-accent hover:text-foreground ' +
  "[&.active]:bg-sidebar-active-bg [&.active]:font-semibold [&.active]:text-sidebar-active " +
  "[&.active]:before:absolute [&.active]:before:inset-y-1.5 [&.active]:before:left-0 [&.active]:before:w-[3px] [&.active]:before:rounded-r-full [&.active]:before:bg-sidebar-active";

function NavLink({
  to,
  params,
  label,
  icon: Icon,
}: {
  to: LinkProps['to'];
  params?: LinkProps['params'];
  label: string;
  icon: LucideIcon;
}) {
  return (
    <Link
      to={to}
      params={params}
      className={navLinkClass}
      activeProps={{ className: 'active' }}
    >
      <Icon className="h-[18px] w-[18px] shrink-0" />
      {label}
    </Link>
  );
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <p className="px-3 pb-1 pt-4 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
      {children}
    </p>
  );
}

function RootLayout() {
  const { user, isLoading, isUnauthenticated } = useAuth();
  const { isOwner, permissions } = useMe();

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center bg-background">
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
    <div className="flex h-screen bg-background">
      <aside className="flex w-64 flex-col border-r border-border bg-sidebar">
        <div className="flex h-14 items-center gap-2 border-b border-border px-4">
          <Link to="/" className="flex items-center gap-2">
            <span className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-sm font-bold text-primary-foreground">
              l
            </span>
            <span className="text-[17px] font-semibold tracking-tight text-foreground">
              le<span className="text-primary">CRM</span>
            </span>
          </Link>
        </div>

        <nav className="flex-1 overflow-y-auto px-2 pb-2">
          <SectionLabel>CRM</SectionLabel>
          <div className="space-y-0.5">
            <NavLink to="/contacts" label="Contacts" icon={Users} />
            <NavLink to="/companies" label="Companies" icon={Building2} />
            <NavLink to="/deals" label="Deals" icon={CircleDollarSign} />
            <NavLink to="/tasks" label="Tasks" icon={CheckSquare} />
          </div>

          {user?.workspace_id && (
            <>
              <SectionLabel>Workspace</SectionLabel>
              <div className="space-y-0.5">
                <NavLink
                  to="/pipeline/$workspaceId"
                  params={{ workspaceId: user.workspace_id }}
                  label="Pipeline"
                  icon={Kanban}
                />
                <NavLink
                  to="/reports/$workspaceId"
                  params={{ workspaceId: user.workspace_id }}
                  label="Reports"
                  icon={BarChart3}
                />
              </div>
            </>
          )}

          <SectionLabel>Configure</SectionLabel>
          <div className="space-y-0.5">
            <NavLink to="/settings" label="Settings" icon={Settings} />
            {permissions.can_write && (
              <NavLink
                to="/settings/custom-fields"
                label="Custom Fields"
                icon={SlidersHorizontal}
              />
            )}
            {isOwner && (
              <NavLink
                to="/settings/members"
                label="Members"
                icon={UserCog}
              />
            )}
          </div>
        </nav>

        <div className="space-y-3 border-t border-border p-3">
          <WorkspaceSwitcher />
          <div className="flex items-center gap-3 rounded-md px-1 py-1">
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
              {(user?.name || user?.email)?.charAt(0)?.toUpperCase() ?? '?'}
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-foreground">
                {user?.name}
              </p>
              <p className="truncate text-xs text-muted-foreground">
                {user?.email}
              </p>
            </div>
            <Button
              asChild
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              title="Help"
            >
              <Link to="/help" aria-label="Help">
                <CircleHelp className="h-4 w-4" />
              </Link>
            </Button>
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

      <main className="flex-1 overflow-auto bg-background">
        <Outlet />
      </main>
    </div>
  );
}

export const Route = createRootRoute({
  component: RootLayout,
});
