import { useAuth } from './use-auth';
import { useWorkspaces } from './use-workspaces';
import {
  selectIntegratorContext,
  type IntegratorContext,
} from '@/lib/integrator';

/**
 * Resolve whether the current user is administering a client workspace as an
 * integrator. Backed by the same accessible-workspaces query the switcher
 * reads, so the banner and the switcher caption never disagree.
 */
export function useIntegratorContext(): IntegratorContext {
  const { user } = useAuth();
  const { workspaces } = useWorkspaces();
  return selectIntegratorContext(workspaces, user?.workspace_slug ?? '');
}
