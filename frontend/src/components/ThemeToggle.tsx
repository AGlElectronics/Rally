import { useEffect, useState } from 'react';
import { Moon, Sun } from 'lucide-react';
import { applyTheme, getAppliedTheme, toggleTheme, type ThemePreference } from '@/lib/theme';
import { cn } from '@/lib/utils';

export function ThemeToggle({ className }: { className?: string }) {
  const [theme, setTheme] = useState<ThemePreference>(getAppliedTheme());

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  return (
    <button
      type="button"
      onClick={() => setTheme(toggleTheme())}
      aria-label="Toggle color theme"
      className={cn(
        'flex w-full items-center gap-2 text-left text-sm px-3 py-2 text-muted-foreground hover:bg-secondary hover:text-foreground transition-colors',
        className
      )}
    >
      {theme === 'dark' ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
      <span>{theme === 'dark' ? 'Light mode' : 'Dark mode'}</span>
    </button>
  );
}
