import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  Outlet,
  RouterProvider,
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router';

import { Button } from '@/components/ui/button';
import './index.css';

const rootRoute = createRootRoute({
  component: () => (
    <div className="min-h-screen bg-background text-foreground">
      <Outlet />
    </div>
  ),
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: function Home() {
    return (
      <main className="container mx-auto flex min-h-screen flex-col items-center justify-center gap-6 py-12">
        <h1 className="text-4xl font-semibold tracking-tight">leCRM</h1>
        <p className="text-muted-foreground">
          Workspace login lives behind the OIDC flow served by the Go API.
        </p>
        <Button asChild>
          <a href="/auth/login">Sign in with Google</a>
        </Button>
      </main>
    );
  },
});

const router = createRouter({
  routeTree: rootRoute.addChildren([indexRoute]),
});

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}

const queryClient = new QueryClient();

const root = document.getElementById('root');
if (!root) throw new Error('#root not found');

ReactDOM.createRoot(root).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </React.StrictMode>,
);
