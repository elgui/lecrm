import { createRoute, Link } from '@tanstack/react-router';
import { Users, Building2, CircleDollarSign } from 'lucide-react';
import { Card, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Route as rootRoute } from './__root';

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: Dashboard,
});

function Dashboard() {
  return (
    <div className="p-8">
      <h1 className="mb-6 text-2xl font-semibold">Dashboard</h1>
      <div className="grid gap-4 md:grid-cols-3">
        <Link to="/contacts">
          <Card className="cursor-pointer transition-shadow hover:shadow-md">
            <CardHeader>
              <div className="flex items-center gap-3">
                <Users className="h-5 w-5 text-muted-foreground" />
                <CardTitle className="text-lg">Contacts</CardTitle>
              </div>
              <CardDescription>Manage your contacts and relationships</CardDescription>
            </CardHeader>
          </Card>
        </Link>
        <Link to="/companies">
          <Card className="cursor-pointer transition-shadow hover:shadow-md">
            <CardHeader>
              <div className="flex items-center gap-3">
                <Building2 className="h-5 w-5 text-muted-foreground" />
                <CardTitle className="text-lg">Companies</CardTitle>
              </div>
              <CardDescription>Track organizations and accounts</CardDescription>
            </CardHeader>
          </Card>
        </Link>
        <Link to="/deals">
          <Card className="cursor-pointer transition-shadow hover:shadow-md">
            <CardHeader>
              <div className="flex items-center gap-3">
                <CircleDollarSign className="h-5 w-5 text-muted-foreground" />
                <CardTitle className="text-lg">Deals</CardTitle>
              </div>
              <CardDescription>Manage your pipeline and revenue</CardDescription>
            </CardHeader>
          </Card>
        </Link>
      </div>
    </div>
  );
}
