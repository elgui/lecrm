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

const ROLE_LABELS: Record<string, string> = {
  member: 'Membre',
  admin: 'Admin',
  owner: 'Propriétaire',
};

function HelpPage() {
  const { role } = useMe();

  return (
    <div className="mx-auto max-w-4xl space-y-6 p-8">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Aide</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Comment fonctionne leCRM, qui peut faire quoi, et comment obtenir de
          l’aide.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Démarrage rapide</CardTitle>
          <CardDescription>Soyez opérationnel en quelques minutes.</CardDescription>
        </CardHeader>
        <CardContent>
          <ol className="list-decimal space-y-2 pl-5 text-sm text-muted-foreground">
            <li>
              Ajoutez un <strong>Contact</strong> ou une{' '}
              <strong>Entreprise</strong> depuis la navigation de gauche.
            </li>
            <li>
              Créez une <strong>Affaire</strong> et suivez sa valeur et son
              étape.
            </li>
            <li>
              Ouvrez le <strong>Pipeline</strong> (Kanban) pour faire glisser
              vos affaires entre les étapes de vente.
            </li>
            <li>
              Notez vos relances dans les <strong>Tâches</strong> pour ne rien
              laisser passer.
            </li>
            <li>
              Utilisez les <strong>Rapports</strong> pour une vue d’ensemble, et{' '}
              <strong>Réglages → Champs personnalisés</strong> pour adapter les
              fiches à votre activité.
            </li>
          </ol>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Comptes &amp; accès</CardTitle>
          <CardDescription>
            Chaque personne a un rôle propre à cet espace de travail.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm text-muted-foreground">
          {role !== 'none' && (
            <p>
              Votre rôle actuel :{' '}
              <Badge variant="secondary">{ROLE_LABELS[role] ?? role}</Badge>
            </p>
          )}
          <ul className="space-y-2">
            <li>
              <strong>Membre</strong> — accès en lecture seule à toutes les
              fiches.
            </li>
            <li>
              <strong>Admin</strong> — tout ce que peut faire un membre, plus la
              création et la modification de fiches et la gestion des champs
              personnalisés.
            </li>
            <li>
              <strong>Propriétaire</strong> — tout ce que peut faire un admin,
              plus l’invitation et le retrait de membres et la modification de
              leurs rôles.
            </li>
          </ul>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Espaces de travail &amp; comptes clients</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm text-muted-foreground">
          <p>
            Chaque client est un espace de travail distinct sur sa propre
            adresse (par exemple <code>client.lecrm.gbconsult.me</code>). Vos
            données, membres et réglages sont isolés par espace.
          </p>
          <p>
            Pour travailler sur les données d’un autre client, connectez-vous à
            l’adresse de ce client. Le changement de compte client depuis
            l’application est prévu mais pas encore disponible.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Besoin d’un coup de main ?</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          <p>
            Contactez votre administrateur leCRM pour les changements d’accès,
            les nouveaux espaces de travail, ou tout ce qui ne fonctionne pas
            comme prévu.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
