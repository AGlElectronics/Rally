import { setAuthHeadersResolver } from '@/api/client';
import { installDevAuth } from '@/auth/dev';
import { bearerHeaders, isOidcAuthMode } from '@/auth/oidc';
import { installSessionSync } from '@/auth/session';

/** Wire the API client's auth header pipeline. Call once at startup. */
export function installAuth(): void {
  if (isOidcAuthMode()) {
    // Rally's API checks the access token's audience (OIDC_AUDIENCE, or
    // ATTENDEE_OIDC_AUDIENCE on the submission site) and refuses ID tokens.
    setAuthHeadersResolver(bearerHeaders);
    installSessionSync();
    return;
  }
  installDevAuth();
}
