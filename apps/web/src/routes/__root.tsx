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
  ShieldCheck,
  type LucideIcon,
} from 'lucide-react';
import { useAuth } from '@/hooks/use-auth';
import { useMe } from '@/hooks/use-me';
import { useIntegratorContext } from '@/hooks/use-integrator-context';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Wordmark } from '@/components/wordmark';
import { WorkspaceSwitcher } from '@/components/WorkspaceSwitcher';
import { MobileTabBar } from '@/components/mobile-tab-bar';

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
    <p className="px-3 pb-1.5 pt-5 text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-400 dark:text-slate-500">
      {children}
    </p>
  );
}

function RootLayout() {
  const { user, isLoading, isUnauthenticated } = useAuth();
  const { isOwner, permissions } = useMe();
  const integrator = useIntegratorContext();

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
      {/* Desktop sidebar — the integrator console is desktop-only, so the full
          nav (incl. config + workspace switcher) lives here and is hidden on
          phones, where the client gets a focused bottom tab bar instead. */}
      <aside className="hidden w-64 flex-col border-r border-border bg-sidebar md:flex">
        <div className="flex h-14 items-center border-b border-border px-4">
          <Link to="/" aria-label="leCRM — accueil">
            <Wordmark />
          </Link>
        </div>

        <nav className="flex-1 overflow-y-auto px-2 pb-2">
          <SectionLabel>CRM</SectionLabel>
          <div className="space-y-0.5">
            <NavLink to="/contacts" label="Contacts" icon={Users} />
            <NavLink to="/companies" label="Entreprises" icon={Building2} />
            <NavLink to="/deals" label="Affaires" icon={CircleDollarSign} />
            <NavLink to="/tasks" label="Tâches" icon={CheckSquare} />
          </div>

          {user?.workspace_id && (
            <>
              <SectionLabel>Espace de travail</SectionLabel>
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
                  label="Rapports"
                  icon={BarChart3}
                />
              </div>
            </>
          )}

          <SectionLabel>Configuration</SectionLabel>
          <div className="space-y-0.5">
            <NavLink to="/settings" label="Réglages" icon={Settings} />
            {permissions.can_write && (
              <NavLink
                to="/settings/custom-fields"
                label="Champs personnalisés"
                icon={SlidersHorizontal}
              />
            )}
            {isOwner && (
              <NavLink
                to="/settings/members"
                label="Membres"
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
              title="Aide"
            >
              <Link to="/help" aria-label="Aide">
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
              title="Déconnexion"
            >
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </aside>

      <main className="flex flex-1 flex-col overflow-hidden bg-background">
        {/* Mobile top header — branding + identity, since the sidebar that
            normally carries them is hidden on phones. */}
        <div className="flex h-14 shrink-0 items-center justify-between border-b border-border px-4 md:hidden">
          <Link to="/" aria-label="leCRM — accueil">
            <Wordmark />
          </Link>
          <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
            {(user?.name || user?.email)?.charAt(0)?.toUpperCase() ?? '?'}
          </div>
        </div>
        {integrator.isIntegrator && (
          <div
            role="status"
            className="flex items-center gap-2.5 border-b border-amber-300 bg-amber-50 px-6 py-2 text-sm text-amber-900 dark:border-amber-500/40 dark:bg-amber-950/50 dark:text-amber-200"
          >
            <ShieldCheck className="h-4 w-4 shrink-0" />
            <span>
              Mode intégrateur · vous administrez le compte client{' '}
              <strong className="font-semibold">{integrator.clientLabel}</strong>
            </span>
          </div>
        )}
        {/* Bottom padding on mobile keeps the last rows clear of the fixed
            tab bar (h-14 + safe-area). */}
        <div className="flex-1 overflow-auto pb-20 md:pb-0">
          <Outlet />
        </div>
      </main>

      <MobileTabBar />
    </div>
  );
}

export const Route = createRootRoute({
  component: RootLayout,
});
