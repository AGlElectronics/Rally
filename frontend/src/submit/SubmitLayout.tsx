import { useQuery } from '@tanstack/react-query';
import { Link, Outlet, useRouterState } from '@tanstack/react-router';
import { CalendarRange, LogIn, LogOut, UserRound } from 'lucide-react';
import { isOidcAuthMode, signInWithOidc, signOutOidc } from '@/auth/oidc';
import { useBrand } from '@/brand/BrandProvider';
import { ThemeToggle } from '@/components/ThemeToggle';
import { Button } from '@/components/ui/Button';
import { cn } from '@/lib/utils';
import { getProfile } from '@/submit/api';
import { useSignedIn } from '@/submit/auth';

export function useProfile(enabled: boolean) {
  return useQuery({ queryKey: ['profile'], queryFn: getProfile, enabled, staleTime: 60_000, retry: false });
}

function Wordmark() {
  const { brand } = useBrand();
  return (
    <Link to="/" className="flex items-center gap-2.5">
      {brand.logoUrl ? (
        <img src={brand.logoUrl} alt="" className="h-8 w-8 rounded-md object-contain" />
      ) : (
        <span className="rounded-pill bg-primary-soft p-1.5">
          <CalendarRange className="h-5 w-5 text-primary" />
        </span>
      )}
      <span className="leading-none">
        <span className="block font-heading text-lg font-extrabold tracking-tight">{brand.productName}</span>
        {brand.organizationName ? (
          <span className="mt-0.5 block text-[11px] font-medium text-muted-foreground">{brand.organizationName}</span>
        ) : null}
      </span>
    </Link>
  );
}

function NavLink({ to, children }: { to: string; children: React.ReactNode }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const active = to === '/' ? pathname === '/' || pathname.startsWith('/calls') : pathname.startsWith(to);
  return (
    <Link
      to={to}
      className={cn(
        'rounded-pill px-3 py-1.5 text-sm font-medium transition-colors',
        active ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:bg-secondary hover:text-foreground',
      )}
    >
      {children}
    </Link>
  );
}

export function SubmitLayout() {
  const { signedIn, ready } = useSignedIn();
  const profile = useProfile(signedIn);
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      <header className="sticky top-0 z-30 border-b border-border bg-card/95 backdrop-blur">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
          <Wordmark />
          <nav className="flex items-center gap-1">
            <NavLink to="/">Open calls</NavLink>
            <NavLink to="/programme">Programme</NavLink>
            {signedIn ? <NavLink to="/submissions">My submissions</NavLink> : null}
          </nav>
          <div className="ml-auto flex items-center gap-2">
            {signedIn ? (
              <>
                <Link
                  to="/profile"
                  className="flex items-center gap-1.5 rounded-pill px-3 py-1.5 text-sm font-medium text-muted-foreground hover:bg-secondary hover:text-foreground"
                >
                  <UserRound className="h-4 w-4" />
                  <span className="max-w-40 truncate">{profile.data?.display_name || 'Profile'}</span>
                </Link>
                {isOidcAuthMode() ? (
                  <Button size="sm" variant="ghost" onClick={() => void signOutOidc().catch(() => undefined)}>
                    <LogOut className="h-4 w-4" />
                    Sign out
                  </Button>
                ) : null}
              </>
            ) : ready ? (
              <Button size="sm" onClick={() => void signInWithOidc(pathname)}>
                <LogIn className="h-4 w-4" />
                Sign in
              </Button>
            ) : null}
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-6 sm:py-8">
        <Outlet />
      </main>
      <footer className="border-t border-border">
        <div className="mx-auto flex max-w-5xl items-center justify-between gap-4 px-4 py-4">
          <span className="text-xs text-muted-foreground">Questions about a call? Contact the organisers.</span>
          <ThemeToggle className="w-auto" />
        </div>
      </footer>
    </div>
  );
}
