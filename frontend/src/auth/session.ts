import type { User } from 'oidc-client-ts';

import { setUnauthorizedHandler } from '@/api/client';
import { getUserManager } from '@/auth/oidc';

/*
 * Keeps every tab on the identity Keycloak currently has. Tokens live only in
 * memory, one copy per tab, so the tabs follow the Keycloak session instead
 * of each other:
 *  - Keycloak session ends (signed out in another tab or app) -> the session
 *    monitor signs this tab out and goes to the login page
 *  - someone else signs in elsewhere -> the session monitor reports a change,
 *    the silent sign-in loads that person, and this tab reloads as them
 *  - the API answers 401 -> back to the login page
 * Without this, a tab kept a stale admin token after a user switch.
 */

export function goToLogin(): void {
  const path = `${window.location.pathname}${window.location.search}`;
  if (path.startsWith('/login') || path.startsWith('/auth/')) return;
  window.location.assign(`/login?redirect=${encodeURIComponent(path)}`);
}

let installed = false;

export function installSessionSync(): void {
  const mgr = getUserManager();
  if (!mgr || installed || typeof window === 'undefined') return;
  installed = true;

  let currentSub: string | undefined;
  mgr.events.addUserLoaded((u: User) => {
    if (currentSub !== undefined && u.profile.sub !== currentSub) {
      window.location.reload();
      return;
    }
    currentSub = u.profile.sub;
  });

  const endSession = () => {
    void mgr.removeUser().finally(goToLogin);
  };
  mgr.events.addUserSignedOut(endSession);
  // A new Keycloak session (signed out and back in elsewhere): adopt it
  // silently; a different person triggers the reload above.
  mgr.events.addUserSessionChanged(() => {
    void mgr.signinSilent().catch(endSession);
  });
  mgr.events.addSilentRenewError((err) => {
    if (/login_required|invalid_grant|session not active/i.test(String(err?.message ?? err))) endSession();
  });

  let redirecting = false;
  setUnauthorizedHandler(() => {
    if (redirecting) return;
    redirecting = true;
    endSession();
  });
}
