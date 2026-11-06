import { useState } from 'react';
import { useNavigate, useRouterState } from '@tanstack/react-router';
import { CalendarRange, LogIn } from 'lucide-react';
import { getDevUser, setDevUser } from '@/auth/dev';
import { isOidcAuthMode, signInWithOidc } from '@/auth/oidc';
import { useBrand } from '@/brand/BrandProvider';
import { LoginBackdrop, loginLook } from '@/components/LoginBackdrop';
import { Button } from '@/components/ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/Card';
import { Input, Label } from '@/components/ui/Input';

/** Attendee sign-in. Accounts here are separate from staff accounts. */
export function SubmitLoginPage() {
  const { brand } = useBrand();
  const navigate = useNavigate();
  const search = useRouterState({ select: (s) => s.location.search as { redirect?: string } });
  const redirect = typeof search.redirect === 'string' && search.redirect.startsWith('/') ? search.redirect : '/';
  const [devName, setDevName] = useState(getDevUser());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const start = async () => {
    setBusy(true);
    try {
      await signInWithOidc(redirect);
    } catch (err) {
      setBusy(false);
      setError(err instanceof Error ? err.message : 'Could not start sign-in.');
    }
  };

  // Den's login caption is written for staff, so the public site shows the picture without it.
  const look = loginLook(brand.loginBackgroundUrl, brand.loginLayout, undefined);
  return (
    <LoginBackdrop look={look} plain="min-h-screen flex items-center justify-center bg-background text-foreground px-4 py-10">
      <Card className="login-card w-full max-w-md">
        <CardHeader>
          <div className="flex items-center gap-2.5">
            <span className="rounded-pill bg-primary-soft p-1.5">
              <CalendarRange className="h-5 w-5 text-primary" />
            </span>
            <CardTitle className="text-xl">{brand.productName}</CardTitle>
          </div>
          <CardDescription>Sign in, or create a free account, to submit and follow your ideas.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {isOidcAuthMode() ? (
            <Button type="button" className="w-full" disabled={busy} onClick={() => void start()}>
              <LogIn className="h-4 w-4" />
              Sign in or create an account
            </Button>
          ) : (
            <form
              className="space-y-3"
              onSubmit={(e) => {
                e.preventDefault();
                if (!devName.trim()) return;
                setDevUser(devName.trim());
                void navigate({ to: redirect });
              }}
            >
              <div>
                <Label htmlFor="dev-user">Dev attendee name</Label>
                <Input id="dev-user" value={devName} onChange={(e) => setDevName(e.target.value)} autoFocus />
              </div>
              <Button type="submit" className="w-full">
                Continue
              </Button>
            </form>
          )}
          <p className="text-xs text-muted-foreground">
            This account is just for submissions; it's separate from any staff or volunteer login.
          </p>
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </CardContent>
      </Card>
    </LoginBackdrop>
  );
}
