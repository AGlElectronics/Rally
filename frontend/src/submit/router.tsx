import { createRootRoute, createRoute, createRouter, Outlet } from '@tanstack/react-router';
import { OidcCallbackPage } from '@/pages/OidcCallbackPage';
import { requireAttendee } from '@/submit/auth';
import { SubmitLayout } from '@/submit/SubmitLayout';
import { InvitePage, ProfilePage } from '@/submit/pages/AccountPages';
import { CallsPage } from '@/submit/pages/CallsPage';
import { ProgrammePage } from '@/submit/pages/ProgrammePage';
import { MySubmissionsPage, NewSubmissionPage, SubmissionPage } from '@/submit/pages/SubmissionPages';
import { SubmitLoginPage } from '@/submit/pages/SubmitLoginPage';

const rootRoute = createRootRoute({ component: () => <Outlet /> });

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  component: SubmitLoginPage,
  validateSearch: (search: Record<string, unknown>) => ({
    redirect: typeof search.redirect === 'string' ? search.redirect : undefined,
  }),
});

const callbackRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/auth/callback',
  component: () => <OidcCallbackPage home="/" />,
});

const siteRoute = createRoute({ getParentRoute: () => rootRoute, id: 'site', component: SubmitLayout });

const guard = ({ location }: { location: { pathname: string } }) => requireAttendee(location.pathname);

const callsRoute = createRoute({ getParentRoute: () => siteRoute, path: '/', component: CallsPage });

const programmeRoute = createRoute({ getParentRoute: () => siteRoute, path: '/programme', component: ProgrammePage });

const newRoute = createRoute({
  getParentRoute: () => siteRoute,
  path: '/calls/$cfpId/new',
  beforeLoad: guard,
  component: NewSubmissionPage,
});

const mineRoute = createRoute({
  getParentRoute: () => siteRoute,
  path: '/submissions',
  beforeLoad: guard,
  component: MySubmissionsPage,
});

const submissionRoute = createRoute({
  getParentRoute: () => siteRoute,
  path: '/submissions/$submissionId',
  beforeLoad: guard,
  component: SubmissionPage,
});

const inviteRoute = createRoute({
  getParentRoute: () => siteRoute,
  path: '/invite/$code',
  beforeLoad: guard,
  component: InvitePage,
});

const profileRoute = createRoute({
  getParentRoute: () => siteRoute,
  path: '/profile',
  beforeLoad: guard,
  component: ProfilePage,
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  callbackRoute,
  siteRoute.addChildren([callsRoute, programmeRoute, newRoute, mineRoute, submissionRoute, inviteRoute, profileRoute]),
]);

export const router = createRouter({ routeTree });

// Type-checked on its own (tsconfig.submit.json), so this registration
// doesn't clash with the staff router's.
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
