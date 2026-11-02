import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  redirect,
} from '@tanstack/react-router';

import { Layout } from '@/components/Layout';
import { AuthCallbackPage } from '@/pages/AuthCallbackPage';
import { DashboardPage } from '@/pages/DashboardPage';
import { LoginPage } from '@/pages/LoginPage';
import { OrganizationsPage } from '@/pages/OrganizationsPage';
import { getUserManager, isOidcAuthMode } from '@/auth/oidc';

// In OIDC mode a route behind the Layout checks there is a cached user
// before letting the component mount. If not, we redirect to /login and
// carry the attempted path so the post-login redirect lands there.
async function requireAuth(fullPath: string) {
  if (!isOidcAuthMode()) {
    return; // dev mode: nothing to check
  }
  const mgr = getUserManager();
  if (!mgr) {
    throw redirect({ to: '/login', search: { redirect: fullPath } });
  }
  const user = await mgr.getUser();
  if (!user || user.expired) {
    throw redirect({ to: '/login', search: { redirect: fullPath } });
  }
}

const rootRoute = createRootRoute({
  component: () => <Outlet />,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  component: LoginPage,
  validateSearch: (search: Record<string, unknown>) => ({
    redirect: typeof search.redirect === 'string' ? search.redirect : undefined,
  }),
});

const authCallbackRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/auth/callback',
  component: AuthCallbackPage,
});

const appLayoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'app',
  component: Layout,
});

const indexRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/',
  beforeLoad: async () => {
    await requireAuth('/dashboard');
    throw redirect({ to: '/dashboard' });
  },
});

const dashboardRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/dashboard',
  beforeLoad: ({ location }) => requireAuth(location.pathname),
  component: DashboardPage,
});

const organizationsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/organizations',
  beforeLoad: ({ location }) => requireAuth(location.pathname),
  component: OrganizationsPage,
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  authCallbackRoute,
  appLayoutRoute.addChildren([indexRoute, dashboardRoute, organizationsRoute]),
]);

export const router = createRouter({ routeTree });

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
