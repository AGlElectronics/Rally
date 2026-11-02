import { useEffect, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { getUserManager } from '@/auth/oidc';
import { oidcCallbackUrl } from '@/auth/captureCallbackUrl';

/**
 * Completes the OIDC Authorization Code flow. We capture the URL at module
 * evaluation time (captureCallbackUrl) because TanStack Router strips
 * unvalidated query params before this component mounts.
 */
export function OidcCallbackPage() {
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const mgr = getUserManager();
    if (!mgr) {
      setError('OIDC is not configured in this build.');
      return;
    }
    mgr
      .signinRedirectCallback(oidcCallbackUrl())
      .then((user) => {
        const dest = (user?.state as string | undefined) || '/dashboard';
        navigate({ to: dest }).catch(() => navigate({ to: '/dashboard' }).catch(() => undefined));
      })
      .catch((e: unknown) => {
        setError(e instanceof Error ? e.message : 'Sign-in failed');
      });
  }, [navigate]);

  return (
    <div className="min-h-screen flex items-center justify-center bg-background text-foreground">
      {error ? (
        <div className="max-w-sm text-center space-y-2">
          <p className="font-semibold text-destructive">Sign-in failed</p>
          <p className="text-sm text-muted-foreground">{error}</p>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">Completing sign-in…</p>
      )}
    </div>
  );
}
