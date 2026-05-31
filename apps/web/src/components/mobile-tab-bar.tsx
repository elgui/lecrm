import * as React from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import type { LinkProps } from '@tanstack/react-router';
import {
  Users,
  Kanban,
  CheckSquare,
  Plus,
  UserPlus,
  CircleDollarSign,
  PhoneCall,
  X,
  type LucideIcon,
} from 'lucide-react';
import { useAuth } from '@/hooks/use-auth';
import { useMe } from '@/hooks/use-me';
import { cn } from '@/lib/utils';

/**
 * Client-facing bottom tab bar — the mobile primary navigation for the TPE
 * client persona, who lives on their phone. Shown only below `md`; the desktop
 * sidebar (integrator console) takes over at `md`+ and stays untouched.
 *
 * Deliberately scoped to the three client surfaces (Contacts, Pipeline,
 * Tâches) plus a central create FAB. Integrator-only surfaces (Réglages,
 * Membres, Champs personnalisés, workspace switcher) are NOT exposed here —
 * the integrator app is desktop-only by design.
 */

const tabBase =
  'flex flex-1 flex-col items-center justify-center gap-0.5 py-1.5 text-[11px] font-medium text-muted-foreground transition-colors';
const tabActive = '[&.active]:text-primary';

function Tab({
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
      className={cn(tabBase, tabActive)}
      activeProps={{ className: 'active' }}
    >
      <Icon className="h-5 w-5" />
      {label}
    </Link>
  );
}

interface CreateAction {
  label: string;
  icon: LucideIcon;
  run: () => void;
}

export function MobileTabBar() {
  const { user } = useAuth();
  const { permissions } = useMe();
  const navigate = useNavigate();
  const [sheetOpen, setSheetOpen] = React.useState(false);
  const workspaceId = user?.workspace_id;

  const actions: CreateAction[] = [
    {
      label: 'Nouveau contact',
      icon: UserPlus,
      run: () => navigate({ to: '/contacts', search: { new: true } }),
    },
    {
      label: 'Nouvelle affaire',
      icon: CircleDollarSign,
      run: () => navigate({ to: '/deals', search: { new: true } }),
    },
    {
      label: 'Enregistrer un appel',
      icon: PhoneCall,
      run: () => navigate({ to: '/tasks' }),
    },
  ];

  return (
    <>
      {/* Spacer so page content never hides behind the fixed bar. */}
      <nav
        aria-label="Navigation principale"
        className="fixed inset-x-0 bottom-0 z-40 flex items-stretch border-t border-border bg-sidebar/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden"
      >
        <Tab to="/contacts" label="Contacts" icon={Users} />
        {workspaceId ? (
          <Tab
            to="/pipeline/$workspaceId"
            params={{ workspaceId }}
            label="Pipeline"
            icon={Kanban}
          />
        ) : (
          <span
            className={cn(tabBase, 'opacity-40')}
            aria-disabled="true"
          >
            <Kanban className="h-5 w-5" />
            Pipeline
          </span>
        )}

        {permissions.can_write ? (
          <button
            type="button"
            onClick={() => setSheetOpen(true)}
            aria-label="Créer"
            className="relative flex w-16 shrink-0 flex-col items-center justify-center"
          >
            <span className="-mt-5 flex h-12 w-12 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-lg ring-4 ring-background transition-transform active:scale-95">
              <Plus className="h-6 w-6" />
            </span>
          </button>
        ) : (
          // Keep the grid balanced for read-only members (no create FAB).
          <span className="w-4 shrink-0" aria-hidden />
        )}

        <Tab to="/tasks" label="Tâches" icon={CheckSquare} />
      </nav>

      {sheetOpen && (
        <CreateSheet
          actions={actions}
          onPick={(action) => {
            setSheetOpen(false);
            action.run();
          }}
          onClose={() => setSheetOpen(false)}
        />
      )}
    </>
  );
}

function CreateSheet({
  actions,
  onPick,
  onClose,
}: {
  actions: CreateAction[];
  onPick: (action: CreateAction) => void;
  onClose: () => void;
}) {
  // Close on Escape for keyboard / accessibility parity with the overlay tap.
  React.useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-50 md:hidden" role="dialog" aria-modal="true">
      <button
        type="button"
        aria-label="Fermer"
        onClick={onClose}
        className="absolute inset-0 bg-foreground/40 backdrop-blur-[1px]"
      />
      <div className="absolute inset-x-0 bottom-0 rounded-t-2xl border-t border-border bg-background pb-[env(safe-area-inset-bottom)] shadow-2xl">
        <div className="flex items-center justify-between px-4 pb-1 pt-3">
          <div className="mx-auto h-1 w-10 rounded-full bg-border" aria-hidden />
        </div>
        <div className="flex items-center justify-between px-4 pb-2">
          <h2 className="text-sm font-semibold text-foreground">Créer</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Fermer"
            className="rounded-md p-1 text-muted-foreground hover:text-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <ul className="px-2 pb-3">
          {actions.map((action) => (
            <li key={action.label}>
              <button
                type="button"
                onClick={() => onPick(action)}
                className="flex w-full items-center gap-3 rounded-lg px-3 py-3 text-left text-sm font-medium text-foreground transition-colors hover:bg-accent active:bg-accent"
              >
                <span className="flex h-9 w-9 items-center justify-center rounded-full bg-primary/10 text-primary">
                  <action.icon className="h-[18px] w-[18px]" />
                </span>
                {action.label}
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
