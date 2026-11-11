import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  redirect,
} from '@tanstack/react-router';

import { Layout } from '@/components/Layout';
import { DashboardPage } from '@/pages/DashboardPage';
import { LoginPage } from '@/pages/LoginPage';
import { OidcCallbackPage } from '@/pages/OidcCallbackPage';
import { OrganizationsPage } from '@/pages/OrganizationsPage';
import { CfPPage } from '@/pages/CfPPage';
import { CfPDetailPage, type CfPTab } from '@/pages/CfPDetailPage';
import { EventTypesPage } from '@/pages/EventTypesPage';
import { SchedulePage, type ScheduleTab } from '@/pages/SchedulePage';
import { SubmissionReviewPage } from '@/pages/SubmissionReviewPage';
import { currentUser, isOidcAuthMode } from '@/auth/oidc';

// In OIDC mode a route behind the Layout checks there is a signed-in user
// (signing in silently after a reload, since tokens live only in memory)
// before letting the component mount. If not, we redirect to /login and
// carry the attempted path so the post-login redirect lands there.
async function requireAuth(fullPath: string) {
  if (!isOidcAuthMode()) {
    return; // dev mode: nothing to check
  }
  if (!(await currentUser())) {
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
  component: OidcCallbackPage,
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

const cfpRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/cfp',
  beforeLoad: ({ location }) => requireAuth(location.pathname),
  component: CfPPage,
});

const cfpDetailRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/cfp/$cfpId',
  beforeLoad: ({ location }) => requireAuth(location.pathname),
  validateSearch: (search: Record<string, unknown>): { tab: CfPTab } => ({
    tab: search.tab === 'form' || search.tab === 'settings' ? search.tab : 'submissions',
  }),
  component: CfPDetailPage,
});

const submissionRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/submissions/$submissionId',
  beforeLoad: ({ location }) => requireAuth(location.pathname),
  component: SubmissionReviewPage,
});

const eventTypesRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/event-types',
  beforeLoad: ({ location }) => requireAuth(location.pathname),
  component: EventTypesPage,
});

const SCHEDULE_TABS: ScheduleTab[] = ['build', 'live', 'versions', 'setup'];

const scheduleRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/schedule',
  beforeLoad: ({ location }) => requireAuth(location.pathname),
  validateSearch: (search: Record<string, unknown>): { tab: ScheduleTab } => ({
    tab: SCHEDULE_TABS.includes(search.tab as ScheduleTab) ? (search.tab as ScheduleTab) : 'build',
  }),
  component: SchedulePage,
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  authCallbackRoute,
  appLayoutRoute.addChildren([
    indexRoute,
    dashboardRoute,
    organizationsRoute,
    cfpRoute,
    cfpDetailRoute,
    submissionRoute,
    eventTypesRoute,
    scheduleRoute,
  ]),
]);

export const router = createRouter({ routeTree });

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
