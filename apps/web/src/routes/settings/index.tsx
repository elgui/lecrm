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
    <div className="mx-auto max-w-4xl space-y-6 p-8">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Réglages</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Gérez l’identité et la configuration de votre espace de travail.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Espace de travail</CardTitle>
          <CardDescription>
            L’identité et la configuration de votre espace de travail.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="ws-name">Nom de l’espace de travail</Label>
            <Input
              id="ws-name"
              defaultValue={user?.workspace_slug ?? ''}
              readOnly
              aria-readonly
            />
            <p className="text-xs text-muted-foreground">
              Le renommage est provisionné par votre intégrateur en v0 ; le
              renommage en libre-service arrive après la v0.
            </p>
          </div>
          <dl className="grid gap-4 sm:grid-cols-2">
            <div>
              <dt className="text-sm font-medium text-muted-foreground">Identifiant de l’espace</dt>
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
          <CardTitle className="text-lg">Personnalisation</CardTitle>
          <CardDescription>
            Logo et couleurs d’accent pour les espaces en marque blanche.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            La personnalisation de la marque est un aperçu en v0 — le code
            source AGPL expose déjà les points d’ancrage de thème ;
            l’interface de gestion arrive après la v0.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
