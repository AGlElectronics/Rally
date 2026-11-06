// Wall-clock helpers for an IANA timezone. The schedule is planned in the
// edition's timezone, whatever the browser's own zone is; the API stores UTC.

const formatters = new Map<string, Intl.DateTimeFormat>();

function partsFormatter(tz: string): Intl.DateTimeFormat {
  let f = formatters.get(tz);
  if (!f) {
    f = new Intl.DateTimeFormat('en-US', {
      timeZone: tz,
      hourCycle: 'h23',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
    formatters.set(tz, f);
  }
  return f;
}

function parts(ms: number, tz: string) {
  const p: Record<string, number> = {};
  for (const x of partsFormatter(tz).formatToParts(new Date(ms))) {
    if (x.type !== 'literal') p[x.type] = Number(x.value);
  }
  return p as { year: number; month: number; day: number; hour: number; minute: number; second: number };
}

function offsetMinutes(ms: number, tz: string): number {
  const p = parts(ms, tz);
  return (Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second) - ms) / 60000;
}

const pad = (n: number) => String(n).padStart(2, '0');

/** Local date (YYYY-MM-DD) and minutes after local midnight of an instant. */
export function zonedParts(iso: string | Date, tz: string): { date: string; minutes: number } {
  const p = parts(new Date(iso).getTime(), tz);
  return { date: `${p.year}-${pad(p.month)}-${pad(p.day)}`, minutes: p.hour * 60 + p.minute };
}

/** The UTC instant (ISO) of a local date + minutes in tz. */
export function zonedToUtc(date: string, minutes: number, tz: string): string {
  const [y, m, d] = date.split('-').map(Number);
  const guess = Date.UTC(y, m - 1, d, 0, minutes);
  let ts = guess - offsetMinutes(guess, tz) * 60000;
  const second = offsetMinutes(ts, tz);
  ts = guess - second * 60000;
  return new Date(ts).toISOString();
}

export function addDays(date: string, n: number): string {
  const [y, m, d] = date.split('-').map(Number);
  const t = new Date(Date.UTC(y, m - 1, d + n));
  return `${t.getUTCFullYear()}-${pad(t.getUTCMonth() + 1)}-${pad(t.getUTCDate())}`;
}

/** Every local date from start to end (inclusive), at most 31. */
export function daysBetween(startIso: string, endIso: string, tz: string): string[] {
  const first = zonedParts(startIso, tz).date;
  const last = zonedParts(endIso, tz).date;
  const out: string[] = [];
  for (let d = first; d <= last && out.length < 31; d = addDays(d, 1)) out.push(d);
  return out;
}

export function minutesLabel(minutes: number): string {
  const m = ((minutes % 1440) + 1440) % 1440;
  return `${pad(Math.floor(m / 60))}:${pad(m % 60)}`;
}

/** "HH:MM" -> minutes, or null. */
export function parseClock(v: string): number | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(v.trim());
  if (!m) return null;
  const h = Number(m[1]);
  const min = Number(m[2]);
  if (h > 24 || min > 59 || (h === 24 && min > 0)) return null;
  return h * 60 + min;
}

export function timeIn(iso: string, tz: string): string {
  return minutesLabel(zonedParts(iso, tz).minutes);
}

export function dayLabel(date: string, opts: Intl.DateTimeFormatOptions = { weekday: 'short', day: 'numeric', month: 'short' }): string {
  const [y, m, d] = date.split('-').map(Number);
  return new Intl.DateTimeFormat(undefined, { ...opts, timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, d)));
}

export function timezones(): string[] {
  const intl = Intl as unknown as { supportedValuesOf?: (k: string) => string[] };
  return intl.supportedValuesOf?.('timeZone') ?? ['UTC', 'Europe/Vienna', 'Europe/Berlin', 'Europe/London', 'America/New_York'];
}
