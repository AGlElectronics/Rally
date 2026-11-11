import { InMemoryWebStorage, UserManager, WebStorageStateStore, type User } from 'oidc-client-ts';
import { runtimeConfig } from '@/runtime-config';

/** True when the SPA was built for Keycloak / OIDC (Bearer tokens). */
export function isOidcAuthMode(): boolean {
  return runtimeConfig().authMode === 'oidc';
}

let manager: UserManager | null | undefined;

/**
 * Lazily construct the oidc-client-ts UserManager. Returns null when OIDC
 * is enabled but misconfigured (missing authority/client id), so callers
 * can surface a clear error instead of a runtime crash.
 *
 * Tokens are kept in memory only (token contract): a script injected into
 * the page cannot read them from localStorage or sessionStorage. After a
 * reload the app signs in again silently through the Keycloak session
 * (currentUser), so nobody has to type a password.
 */
export function getUserManager(): UserManager | null {
  if (manager !== undefined) {
    return manager;
  }
  const authority = runtimeConfig().oidcAuthority;
  const client_id = runtimeConfig().oidcClientId;
  if (!authority || !client_id) {
    manager = null;
    return null;
  }
  if (typeof window === 'undefined') {
    manager = null;
    return null;
  }
  const scope = runtimeConfig().oidcScope;
  manager = new UserManager({
    authority,
    client_id,
    redirect_uri: `${window.location.origin}/auth/callback`,
    post_logout_redirect_uri: `${window.location.origin}/login`,
    response_type: 'code',
    scope,
    automaticSilentRenew: true,
    silent_redirect_uri: `${window.location.origin}/silent-callback.html`,
    monitorSession: true,
    userStore: new WebStorageStateStore({ store: new InMemoryWebStorage() }),
  });
  manager.events.addUserLoaded(() => setSessionHint(true));
  manager.events.addUserSignedOut(() => setSessionHint(false));
  return manager;
}

// Whether this browser signed in before. Not a token: it only tells the
// submission site whether a silent sign-in is worth trying for a visitor
// who may never have had an account.
const SESSION_HINT_KEY = 'rally.oidc.signedIn';

function setSessionHint(on: boolean): void {
  try {
    if (on) window.localStorage.setItem(SESSION_HINT_KEY, '1');
    else window.localStorage.removeItem(SESSION_HINT_KEY);
  } catch {
    // Storage blocked: silent sign-in is then tried only where required.
  }
}

function hadSession(): boolean {
  try {
    return window.localStorage.getItem(SESSION_HINT_KEY) === '1';
  } catch {
    return false;
  }
}

let restoring: Promise<User | null> | null = null;

/**
 * The signed-in user, or null. Without a valid user in memory (after a
 * reload, or when the access token expired) it signs in silently through the
 * Keycloak session; with `silent: 'if-signed-in-before'` only when this
 * browser signed in before. Concurrent callers share one attempt.
 */
export async function currentUser(opts: { silent?: true | 'if-signed-in-before' } = { silent: true }): Promise<User | null> {
  const mgr = getUserManager();
  if (!mgr) return null;
  const user = await mgr.getUser();
  if (user && !user.expired) return user;
  if (!opts.silent || (opts.silent === 'if-signed-in-before' && !hadSession())) return null;
  restoring ??= mgr
    .signinSilent()
    .catch(() => {
      setSessionHint(false);
      return null;
    })
    .finally(() => {
      restoring = null;
    });
  return restoring;
}

/** The Authorization header for the API: the access token, never the ID token. */
export async function bearerHeaders(): Promise<Record<string, string>> {
  const user = await currentUser();
  if (!user?.access_token) return {};
  return { Authorization: `Bearer ${user.access_token}` };
}

export async function signInWithOidc(returnPath: string): Promise<void> {
  const mgr = getUserManager();
  if (!mgr) {
    throw new Error('OIDC is not configured (set oidcAuthority and oidcClientId in /config.json).');
  }
  const safePath = returnPath.startsWith('/') ? returnPath : '/dashboard';
  try {
    await mgr.signinRedirect({ state: safePath });
  } catch (err) {
    // fetch() rejects with a bare TypeError ("Failed to fetch") when the login
    // server is down or its certificate isn't trusted.
    if (err instanceof TypeError) {
      throw new Error(
        `Can't reach the login server (${new URL(mgr.settings.authority).host}). It may be down or its certificate may have expired.`,
      );
    }
    throw err;
  }
}

export async function signOutOidc(): Promise<void> {
  const mgr = getUserManager();
  if (!mgr) {
    return;
  }
  setSessionHint(false);
  await mgr.signoutRedirect();
}
