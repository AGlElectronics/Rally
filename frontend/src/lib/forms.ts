// Submission form model shared by the staff app (form builder, review) and
// the public submission site. Mirrors internal/events/forms.go.

export type FieldType =
  | 'short_text'
  | 'long_text'
  | 'number'
  | 'duration'
  | 'single_choice'
  | 'multi_choice'
  | 'checkbox'
  | 'email'
  | 'url'
  | 'datetime'
  | 'file';

export type FormField = {
  id: string;
  type: FieldType;
  label: string;
  help?: string;
  required?: boolean;
  options?: string[];
  min?: number;
  max?: number;
  accept?: string[];
  max_files?: number;
  max_size_mb?: number;
  archived?: boolean;
};

export type Form = { fields: FormField[] };

export type AnswerValue = string | number | boolean | string[];
export type Answers = Record<string, AnswerValue | undefined>;

export const FIELD_TYPES: { value: FieldType; label: string; hint: string }[] = [
  { value: 'short_text', label: 'Short text', hint: 'One line, e.g. a name or tagline' },
  { value: 'long_text', label: 'Long text', hint: 'Paragraphs, e.g. a description' },
  { value: 'single_choice', label: 'Single choice', hint: 'Pick one option' },
  { value: 'multi_choice', label: 'Multiple choice', hint: 'Pick any number of options' },
  { value: 'checkbox', label: 'Checkbox', hint: 'Yes / no, e.g. "I agree to…"' },
  { value: 'number', label: 'Number', hint: 'A number, e.g. group size' },
  { value: 'duration', label: 'Duration', hint: 'Minutes' },
  { value: 'email', label: 'Email', hint: 'An email address' },
  { value: 'url', label: 'Link', hint: 'A web address' },
  { value: 'datetime', label: 'Date and time', hint: 'A specific moment' },
  { value: 'file', label: 'File upload', hint: 'Images, PDFs, riders…' },
];

export const fieldTypeLabel = (t: FieldType) => FIELD_TYPES.find((f) => f.value === t)?.label ?? t;

/** Built-in fields that live outside the form because scheduling needs them. */
export const SYSTEM_FIELDS = [
  { id: 'title', label: 'Title', note: 'Always asked, required' },
  { id: 'event_type', label: 'Event type', note: 'Asked when the call offers event types' },
  { id: 'duration', label: 'Requested length', note: 'Optional; defaults to the event type' },
  { id: 'people', label: 'People involved', note: 'Co-presenters with attendee accounts' },
] as const;

export const SUBMISSION_STATES = ['draft', 'submitted', 'accepted', 'rejected', 'withdrawn'] as const;
export type SubmissionState = (typeof SUBMISSION_STATES)[number];

export const DEFAULT_STATUS_LABELS: Record<SubmissionState, string> = {
  draft: 'Draft',
  submitted: 'Submitted',
  accepted: 'Accepted',
  rejected: 'Not accepted',
  withdrawn: 'Withdrawn',
};

export function statusLabel(state: SubmissionState, labels?: Record<string, string>): string {
  return labels?.[state] || DEFAULT_STATUS_LABELS[state] || state;
}

/** A field id from a label: "Tech rider" -> "tech_rider". */
export function slugifyFieldId(label: string, taken: Set<string>): string {
  let base = label
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .slice(0, 32);
  if (!/^[a-z]/.test(base)) base = `q_${base}`.replace(/_+$/, '');
  if (base === 'q' || base === '') base = 'question';
  const reserved = new Set(['title', 'event_type', 'duration', 'people']);
  let id = base;
  for (let i = 2; taken.has(id) || reserved.has(id); i++) id = `${base}_${i}`;
  return id;
}

export function isEmptyAnswer(v: AnswerValue | undefined): boolean {
  return v === undefined || v === '' || v === false || (Array.isArray(v) && v.length === 0);
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}
