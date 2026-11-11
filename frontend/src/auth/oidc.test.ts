// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { User } from 'oidc-client-ts';

vi.mock('@/runtime-config', () => ({
  runtimeConfig: () => ({
    authMode: 'oidc',
    oidcAuthority: 'https://auth.example.org/realms/staff',
    oidcClientId: 'events-spa',
    oidcScope: 'openid profile email',
    apiBaseUrl: '',
    productName: 'Rally',
    publicScheduleKey: '',
  }),
}));

function signedIn(sub = 'u1') {
  return new User({
    id_token: 'the-id-token',
    access_token: 'the-access-token',
    token_type: 'Bearer',
    profile: { sub, iss: 'https://auth.example.org/realms/staff', aud: 'events-spa', exp: 0, iat: 0 },
    expires_at: Math.floor(Date.now() / 1000) + 300,
  });
}

// A fresh module per test: the UserManager is module state.
async function load() {
  vi.resetModules();
  return import('./oidc');
}

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('token storage', () => {
  it('keeps tokens in memory, never in localStorage or sessionStorage', async () => {
    const { getUserManager } = await load();
    const mgr = getUserManager()!;
    await mgr.storeUser(signedIn());
    expect((await mgr.getUser())?.access_token).toBe('the-access-token');
    const stored = [window.localStorage, window.sessionStorage].flatMap((s) =>
      Array.from({ length: s.length }, (_, i) => `${s.key(i)}=${s.getItem(s.key(i) ?? '')}`),
    );
    expect(stored.join('\n')).not.toContain('the-access-token');
    expect(stored.join('\n')).not.toContain('the-id-token');
  });
});

describe('API bearer', () => {
  it('sends the access token, not the ID token', async () => {
    const { getUserManager, bearerHeaders } = await load();
    await getUserManager()!.storeUser(signedIn());
    expect(await bearerHeaders()).toEqual({ Authorization: 'Bearer the-access-token' });
  });

  it('signs in silently once after a reload and shares the attempt', async () => {
    const { getUserManager, currentUser } = await load();
    const mgr = getUserManager()!;
    const silent = vi.spyOn(mgr, 'signinSilent').mockImplementation(async () => {
      const u = signedIn('u2');
      await mgr.storeUser(u);
      return u;
    });
    const [a, b] = await Promise.all([currentUser(), currentUser()]);
    expect(silent).toHaveBeenCalledTimes(1);
    expect(a?.profile.sub).toBe('u2');
    expect(b?.profile.sub).toBe('u2');
  });

  it('gives up quietly when there is no Keycloak session', async () => {
    const { getUserManager, currentUser, bearerHeaders } = await load();
    vi.spyOn(getUserManager()!, 'signinSilent').mockRejectedValue(new Error('login_required'));
    expect(await currentUser()).toBeNull();
    expect(await bearerHeaders()).toEqual({});
  });

  it('only tries a silent sign-in for a returning visitor when asked to', async () => {
    const { getUserManager, currentUser } = await load();
    const silent = vi.spyOn(getUserManager()!, 'signinSilent').mockRejectedValue(new Error('login_required'));
    expect(await currentUser({ silent: 'if-signed-in-before' })).toBeNull();
    expect(silent).not.toHaveBeenCalled();
    window.localStorage.setItem('rally.oidc.signedIn', '1');
    await currentUser({ silent: 'if-signed-in-before' });
    expect(silent).toHaveBeenCalledTimes(1);
    expect(window.localStorage.getItem('rally.oidc.signedIn')).toBeNull();
  });
});
