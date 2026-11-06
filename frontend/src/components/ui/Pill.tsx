import { cn } from '@/lib/utils';
import { statusLabel, type SubmissionState } from '@/lib/forms';

const TONES = {
  neutral: 'bg-secondary text-secondary-foreground',
  primary: 'bg-primary-soft text-primary',
  success: 'bg-success/15 text-success',
  warning: 'bg-warning/15 text-warning',
  danger: 'bg-destructive/15 text-destructive',
} as const;

export type Tone = keyof typeof TONES;

export function Pill({ tone = 'neutral', className, ...rest }: React.HTMLAttributes<HTMLSpanElement> & { tone?: Tone }) {
  return (
    <span
      className={cn('inline-flex items-center gap-1 rounded-pill px-2.5 py-0.5 text-xs font-semibold whitespace-nowrap', TONES[tone], className)}
      {...rest}
    />
  );
}

const STATE_TONES: Record<SubmissionState, Tone> = {
  draft: 'neutral',
  submitted: 'primary',
  accepted: 'success',
  rejected: 'danger',
  withdrawn: 'warning',
};

export function StatePill({ state, labels }: { state: SubmissionState; labels?: Record<string, string> }) {
  return <Pill tone={STATE_TONES[state]}>{statusLabel(state, labels)}</Pill>;
}

export function ColorDot({ color, className }: { color: string; className?: string }) {
  return <span className={cn('inline-block h-2.5 w-2.5 shrink-0 rounded-full', className)} style={{ backgroundColor: color }} />;
}

export const selectClasses =
  'h-9 rounded-md border border-input bg-background px-3 text-sm transition-colors focus-visible:outline-none focus-visible:border-primary focus-visible:ring-[3px] focus-visible:ring-primary-soft disabled:opacity-50';
