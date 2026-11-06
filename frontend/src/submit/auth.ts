import { useEffect, useState } from 'react';
import { redirect } from '@tanstack/react-router';
import { getUserManager, isOidcAuthMode } from '@/auth/oidc';

/**
 * Whether an attendee is signed in. Browsing open calls works without an
 * account, so the submission site only asks the API for personal data once
 * this is true (an anonymous call would answer 401 and bounce to sign-in).
 */
export function useSignedIn(): { signedIn: boolean; ready: boolean } {
  const [state, setState] = useState({ signedIn: !isOidcAuthMode(), ready: !isOidcAuthMode() });
  useEffect(() => {
    const mgr = isOidcAuthMode() ? getUserManager() : null;
    if (!mgr) return;
    const sync = () =>
      void mgr.getUser().then((u) => setState({ signedIn: !!u && !u.expired, ready: true }));
    sync();
    mgr.events.addUserLoaded(sync);
    mgr.events.addUserUnloaded(sync);
    return () => {
      mgr.events.removeUserLoaded(sync);
      mgr.events.removeUserUnloaded(sync);
    };
  }, []);
  return state;
}

/** Route guard: send anonymous visitors to sign-in, then back here. */
export async function requireAttendee(fullPath: string) {
  if (!isOidcAuthMode()) return;
  const user = await getUserManager()?.getUser();
  if (!user || user.expired) {
    throw redirect({ to: '/login', search: { redirect: fullPath } });
  }
}
