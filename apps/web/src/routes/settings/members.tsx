import { createRoute } from '@tanstack/react-router';
import { useState } from 'react';
import { useMe } from '@/hooks/use-me';
import {
  useMembers,
  useInviteMember,
  useUpdateMemberRole,
  useRemoveMember,
} from '@/hooks/use-members';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card';
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from '@/components/ui/table';
import type { Role } from '@/lib/types';
import { Route as rootRoute } from '../__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/members',
  component: MembersPage,
});

const ROLES: Role[] = ['member', 'admin', 'owner'];

// French display label for a role. The enum value stays on the wire.
const ROLE_LABELS: Record<Role, string> = {
  member: 'Membre',
  admin: 'Admin',
  owner: 'Propriétaire',
  none: 'Aucun',
};
function roleLabel(role: Role): string {
  return ROLE_LABELS[role] ?? role;
}

function MembersPage() {
  const { me, isOwner, isLoading: meLoading } = useMe();

  if (meLoading) {
    return (
      <div className="space-y-4 p-8">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  // Owner-only page: members and admins are not allowed here.
  if (!isOwner) {
    return (
      <div className="mx-auto max-w-5xl p-8">
        <h1 className="mb-2 text-xl font-semibold tracking-tight">Membres</h1>
        <p className="text-destructive">
          Seuls les propriétaires de l’espace peuvent gérer les membres.
        </p>
      </div>
    );
  }

  return <MembersManager currentUserId={me?.user_id ?? ''} />;
}

function MembersManager({ currentUserId }: { currentUserId: string }) {
  const { data: members, isLoading, error } = useMembers();
  const invite = useInviteMember();
  const updateRole = useUpdateMemberRole();
  const remove = useRemoveMember();

  const [email, setEmail] = useState('');
  const [role, setRole] = useState<Role>('member');

  const onInvite = (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim()) return;
    invite.mutate(
      { email: email.trim(), role },
      { onSuccess: () => setEmail('') },
    );
  };

  return (
    <div className="mx-auto max-w-5xl space-y-6 p-8">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Membres</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Invitez vos collègues et gérez leur accès à cet espace de travail.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Inviter un membre</CardTitle>
          <CardDescription>
            Envoyez une invitation à l’espace de travail. La personne rejoint
            avec le rôle sélectionné.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onInvite} className="flex flex-wrap items-end gap-3">
            <div className="space-y-2">
              <Label htmlFor="invite-email">E-mail</Label>
              <Input
                id="invite-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="teammate@company.com"
                className="w-72"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="invite-role">Rôle</Label>
              <select
                id="invite-role"
                value={role}
                onChange={(e) => setRole(e.target.value as Role)}
                className="h-10 rounded-md border border-input bg-card px-3 text-sm shadow-xs focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/25"
              >
                {ROLES.map((r) => (
                  <option key={r} value={r}>
                    {roleLabel(r)}
                  </option>
                ))}
              </select>
            </div>
            <Button type="submit" disabled={invite.isPending || !email.trim()}>
              {invite.isPending ? 'Invitation…' : 'Envoyer l’invitation'}
            </Button>
          </form>
          {invite.isError && (
            <p className="mt-2 text-sm text-destructive">
              {(invite.error as Error).message}
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Membres de l’espace de travail</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading && <Skeleton className="h-32 w-full" />}
          {error && (
            <p className="text-destructive">
              Échec du chargement des membres : {(error as Error).message}
            </p>
          )}
          {members && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>E-mail</TableHead>
                  <TableHead>Rôle</TableHead>
                  <TableHead>Statut</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {members.map((m) => {
                  const isSelf = m.user_id === currentUserId;
                  return (
                    <TableRow key={m.user_id}>
                      <TableCell className="font-medium">
                        {m.email ?? m.user_id.slice(0, 8) + '…'}
                        {isSelf && (
                          <span className="ml-2 text-xs text-muted-foreground">
                            (vous)
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <select
                          aria-label={`Rôle pour ${m.email ?? m.user_id}`}
                          value={m.role}
                          disabled={isSelf || updateRole.isPending}
                          onChange={(e) =>
                            updateRole.mutate({
                              userId: m.user_id,
                              role: e.target.value as Role,
                            })
                          }
                          className="h-9 rounded-md border border-input bg-card px-2 text-sm shadow-xs focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/25 disabled:opacity-50"
                        >
                          {ROLES.map((r) => (
                            <option key={r} value={r}>
                              {roleLabel(r)}
                            </option>
                          ))}
                        </select>
                      </TableCell>
                      <TableCell>
                        {m.pending ? (
                          <Badge variant="warning">En attente</Badge>
                        ) : (
                          <Badge variant="success">Actif</Badge>
                        )}
                      </TableCell>
                      <TableCell className="text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={isSelf || remove.isPending}
                          onClick={() => remove.mutate(m.user_id)}
                          className="text-muted-foreground hover:text-destructive"
                          title={
                            isSelf
                              ? 'Vous ne pouvez pas vous retirer vous-même'
                              : 'Retirer le membre'
                          }
                        >
                          Retirer
                        </Button>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
