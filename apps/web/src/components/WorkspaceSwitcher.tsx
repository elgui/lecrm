import { type Dispatch, type SetStateAction, useEffect, useRef, useState } from 'react';
import { Check, ChevronsUpDown } from 'lucide-react';
import { useAuth } from '@/hooks/use-auth';
import { useWorkspaces } from '@/hooks/use-workspaces';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

// French display label for a workspace membership role token.
function wsRoleLabel(role: string): string {
  const map: Record<string, string> = {
    integrator: 'intégrateur',
    owner: 'propriétaire',
    admin: 'admin',
    member: 'membre',
  };
  return map[role] ?? role;
}

export function WorkspaceSwitcher() {
  const { user } = useAuth();
  const { workspaces } = useWorkspaces();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // Hide when there is only one (or zero) accessible workspace — single-workspace
  // users have nothing to switch to.
  if (!workspaces || workspaces.length <= 1) {
    return null;
  }

  const currentSlug = user?.workspace_slug ?? '';
  const isIntegrator =
    currentSlug !== '' &&
    workspaces.some((ws) => ws.slug === currentSlug && ws.role === 'integrator');

  const triggerLabel = isIntegrator
    ? `GB Consult · administre ${currentSlug}`
    : currentSlug;

  const dropdownLabel = isIntegrator ? 'Comptes clients' : 'Vos espaces de travail';

  return (
    <WorkspaceSwitcherInner
      containerRef={containerRef}
      open={open}
      setOpen={setOpen}
      triggerLabel={triggerLabel}
      dropdownLabel={dropdownLabel}
      workspaces={workspaces}
      currentSlug={currentSlug}
    />
  );
}

// Extracted inner component to allow hooks-before-early-return ordering without
// the early return appearing before hooks (which is illegal in React).
interface InnerProps {
  containerRef: React.RefObject<HTMLDivElement | null>;
  open: boolean;
  setOpen: Dispatch<SetStateAction<boolean>>;
  triggerLabel: string;
  dropdownLabel: string;
  workspaces: { slug: string; role: string; url: string }[];
  currentSlug: string;
}

function WorkspaceSwitcherInner({
  containerRef,
  open,
  setOpen,
  triggerLabel,
  dropdownLabel,
  workspaces,
  currentSlug,
}: InnerProps) {
  // Close the dropdown when the user clicks outside the container.
  useEffect(() => {
    if (!open) return;
    function handleOutside(e: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    }
    document.addEventListener('mousedown', handleOutside);
    return () => document.removeEventListener('mousedown', handleOutside);
  }, [open, containerRef, setOpen]);

  return (
    <div ref={containerRef} className="relative">
      <Button
        variant="outline"
        size="sm"
        className="max-w-[200px] truncate"
        onClick={() => setOpen((prev) => !prev)}
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        <span className="truncate">{triggerLabel}</span>
        <ChevronsUpDown className="ml-1 h-3 w-3 shrink-0 opacity-50" />
      </Button>

      {open && (
        <div
          className={cn(
            'absolute right-0 z-50 mt-1 min-w-[180px] rounded-md border bg-popover text-popover-foreground shadow-md',
            'animate-in fade-in-0 zoom-in-95',
          )}
          role="listbox"
          aria-label={dropdownLabel}
        >
          {/* Label */}
          <div className="px-2 py-1.5 text-xs font-semibold text-muted-foreground">
            {dropdownLabel}
          </div>

          {/* Separator */}
          <div className="-mx-0 my-1 h-px bg-muted" />

          {/* Workspace items */}
          {workspaces.map((ws) => {
            const isCurrent = ws.slug === currentSlug;
            return isCurrent ? (
              // Current workspace: not a link, visually marked
              <div
                key={ws.slug}
                role="option"
                aria-selected={true}
                className="flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm opacity-70"
              >
                <Check className="h-3 w-3 shrink-0" />
                <span className="flex-1 truncate">{ws.slug}</span>
                <span className="ml-1 text-xs text-muted-foreground">
                  {wsRoleLabel(ws.role)}
                </span>
              </div>
            ) : (
              // Other workspace: full-page navigation anchor
              <a
                key={ws.slug}
                href={ws.url}
                role="option"
                aria-selected={false}
                onClick={() => setOpen(false)}
                className={cn(
                  'flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm',
                  'cursor-pointer hover:bg-accent hover:text-accent-foreground',
                  'focus:outline-none focus:bg-accent focus:text-accent-foreground',
                )}
              >
                <span className="h-3 w-3 shrink-0" aria-hidden />
                <span className="flex-1 truncate">{ws.slug}</span>
                <span className="ml-1 text-xs text-muted-foreground">
                  {wsRoleLabel(ws.role)}
                </span>
              </a>
            );
          })}
        </div>
      )}
    </div>
  );
}
