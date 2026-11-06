import { useRef, useState } from 'react';
import { FileText, Loader2, Paperclip, Upload, X } from 'lucide-react';
import { Input, Label } from '@/components/ui/Input';
import { cn } from '@/lib/utils';
import { formatBytes, type AnswerValue, type Answers, type FormField } from '@/lib/forms';
import type { UploadedFile } from '@/lib/upload';

export const textareaClasses =
  'w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground transition-colors focus-visible:outline-none focus-visible:border-primary focus-visible:ring-[3px] focus-visible:ring-primary-soft disabled:cursor-not-allowed disabled:opacity-50';

type Props = {
  fields: FormField[];
  answers: Answers;
  onChange: (id: string, value: AnswerValue | undefined) => void;
  /** Validation issues keyed like the API ("answers.<id>"). */
  errors?: Record<string, string>;
  /** Files already known (for showing names of uploaded ids). */
  files?: UploadedFile[];
  /** Upload handler; absent = uploads disabled (e.g. a staff preview). */
  onUpload?: (fieldId: string, file: File) => Promise<UploadedFile>;
  onOpenFile?: (fileId: string) => void;
  disabled?: boolean;
};

/** Renders the configurable part of a submission form. */
export function FormFields({ fields, answers, onChange, errors, files, onUpload, onOpenFile, disabled }: Props) {
  const visible = fields.filter((f) => !f.archived);
  if (visible.length === 0) return null;
  return (
    <div className="space-y-5">
      {visible.map((f) => (
        <FieldInput
          key={f.id}
          field={f}
          value={answers[f.id]}
          onChange={(v) => onChange(f.id, v)}
          error={errors?.[`answers.${f.id}`]}
          files={files}
          onUpload={onUpload}
          onOpenFile={onOpenFile}
          disabled={disabled}
        />
      ))}
    </div>
  );
}

function FieldLabel({ field, htmlFor }: { field: FormField; htmlFor?: string }) {
  return (
    <Label htmlFor={htmlFor}>
      {field.label}
      {field.required ? <span className="text-destructive"> *</span> : null}
    </Label>
  );
}

function FieldInput({
  field,
  value,
  onChange,
  error,
  files,
  onUpload,
  onOpenFile,
  disabled,
}: {
  field: FormField;
  value: AnswerValue | undefined;
  onChange: (v: AnswerValue | undefined) => void;
  error?: string;
  files?: UploadedFile[];
  onUpload?: Props['onUpload'];
  onOpenFile?: Props['onOpenFile'];
  disabled?: boolean;
}) {
  const id = `field-${field.id}`;
  const help = field.help ? <p className="text-xs text-muted-foreground mt-1 whitespace-pre-line">{field.help}</p> : null;
  const err = error ? <p className="text-xs text-destructive mt-1">{error}</p> : null;
  const str = typeof value === 'string' ? value : '';

  switch (field.type) {
    case 'long_text':
      return (
        <div>
          <FieldLabel field={field} htmlFor={id} />
          <textarea
            id={id}
            className={cn(textareaClasses, 'min-h-32')}
            value={str}
            maxLength={field.max ?? 10000}
            disabled={disabled}
            onChange={(e) => onChange(e.target.value)}
          />
          {help}
          {err}
        </div>
      );
    case 'single_choice':
      return (
        <fieldset>
          <FieldLabel field={field} />
          <div className="space-y-1.5">
            {(field.options ?? []).filter((o) => o.trim()).map((o) => (
              <label key={o} className="flex items-center gap-2 text-sm">
                <input
                  type="radio"
                  name={id}
                  checked={value === o}
                  disabled={disabled}
                  onChange={() => onChange(o)}
                  className="accent-primary"
                />
                {o}
              </label>
            ))}
          </div>
          {help}
          {err}
        </fieldset>
      );
    case 'multi_choice': {
      const picked = Array.isArray(value) ? value : [];
      return (
        <fieldset>
          <FieldLabel field={field} />
          <div className="space-y-1.5">
            {(field.options ?? []).filter((o) => o.trim()).map((o) => (
              <label key={o} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={picked.includes(o)}
                  disabled={disabled}
                  onChange={(e) => onChange(e.target.checked ? [...picked, o] : picked.filter((p) => p !== o))}
                  className="accent-primary"
                />
                {o}
              </label>
            ))}
          </div>
          {help}
          {err}
        </fieldset>
      );
    }
    case 'checkbox':
      return (
        <div>
          <label className="flex items-start gap-2 text-sm font-medium">
            <input
              id={id}
              type="checkbox"
              checked={value === true}
              disabled={disabled}
              onChange={(e) => onChange(e.target.checked)}
              className="mt-0.5 accent-primary"
            />
            <span>
              {field.label}
              {field.required ? <span className="text-destructive"> *</span> : null}
            </span>
          </label>
          {help}
          {err}
        </div>
      );
    case 'number':
    case 'duration':
      return (
        <div>
          <FieldLabel field={field} htmlFor={id} />
          <div className="flex items-center gap-2">
            <Input
              id={id}
              type="number"
              className="max-w-40"
              value={typeof value === 'number' ? String(value) : ''}
              min={field.min}
              max={field.max}
              step={field.type === 'duration' ? 5 : 'any'}
              disabled={disabled}
              onChange={(e) => onChange(e.target.value === '' ? undefined : Number(e.target.value))}
            />
            {field.type === 'duration' ? <span className="text-sm text-muted-foreground">minutes</span> : null}
          </div>
          {help}
          {err}
        </div>
      );
    case 'datetime':
      return (
        <div>
          <FieldLabel field={field} htmlFor={id} />
          <Input
            id={id}
            type="datetime-local"
            className="max-w-64"
            value={str ? toLocalInput(str) : ''}
            disabled={disabled}
            onChange={(e) => onChange(e.target.value ? new Date(e.target.value).toISOString() : undefined)}
          />
          {help}
          {err}
        </div>
      );
    case 'file':
      return (
        <FileField
          field={field}
          value={Array.isArray(value) ? value : []}
          onChange={onChange}
          error={error}
          help={help}
          files={files}
          onUpload={onUpload}
          onOpenFile={onOpenFile}
          disabled={disabled}
        />
      );
    default:
      return (
        <div>
          <FieldLabel field={field} htmlFor={id} />
          <Input
            id={id}
            type={field.type === 'email' ? 'email' : field.type === 'url' ? 'url' : 'text'}
            value={str}
            maxLength={field.max ?? 200}
            placeholder={field.type === 'url' ? 'https://' : undefined}
            disabled={disabled}
            onChange={(e) => onChange(e.target.value)}
          />
          {help}
          {err}
        </div>
      );
  }
}

function toLocalInput(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function FileField({
  field,
  value,
  onChange,
  error,
  help,
  files,
  onUpload,
  onOpenFile,
  disabled,
}: {
  field: FormField;
  value: string[];
  onChange: (v: AnswerValue | undefined) => void;
  error?: string;
  help: React.ReactNode;
  files?: UploadedFile[];
  onUpload?: Props['onUpload'];
  onOpenFile?: Props['onOpenFile'];
  disabled?: boolean;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [local, setLocal] = useState<UploadedFile[]>([]);
  const known = [...(files ?? []), ...local];
  const max = field.max_files || 1;
  const maxMb = field.max_size_mb || 10;

  const pick = async (list: FileList | null) => {
    if (!list || !onUpload) return;
    setUploadError(null);
    const chosen = Array.from(list).slice(0, Math.max(0, max - value.length));
    const tooBig = chosen.find((f) => f.size > maxMb * 1024 * 1024);
    if (tooBig) {
      setUploadError(`${tooBig.name} is larger than ${maxMb} MB.`);
      return;
    }
    setBusy(true);
    let next = [...value];
    try {
      for (const f of chosen) {
        const up = await onUpload(field.id, f);
        setLocal((l) => [...l, up]);
        next = [...next, up.id];
        onChange(next);
      }
    } catch (e) {
      setUploadError(e instanceof Error ? e.message : 'Upload failed.');
    } finally {
      setBusy(false);
      if (input.current) input.current.value = '';
    }
  };

  return (
    <div>
      <FieldLabel field={field} />
      <ul className="space-y-1.5">
        {value.map((id) => {
          const f = known.find((k) => k.id === id);
          return (
            <li key={id} className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm">
              <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
              <button
                type="button"
                className="min-w-0 flex-1 truncate text-left hover:underline disabled:no-underline"
                disabled={!onOpenFile}
                onClick={() => onOpenFile?.(id)}
              >
                {f?.filename ?? 'Uploaded file'}
              </button>
              {f ? <span className="text-xs text-muted-foreground">{formatBytes(f.size_bytes)}</span> : null}
              {!disabled ? (
                <button
                  type="button"
                  aria-label="Remove file"
                  className="rounded-pill p-1 text-muted-foreground hover:bg-secondary hover:text-foreground"
                  onClick={() => onChange(value.filter((v) => v !== id))}
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              ) : null}
            </li>
          );
        })}
      </ul>
      {value.length < max && !disabled ? (
        <>
          <input
            ref={input}
            type="file"
            className="hidden"
            multiple={max - value.length > 1}
            accept={(field.accept ?? []).join(',') || undefined}
            onChange={(e) => void pick(e.target.files)}
          />
          <button
            type="button"
            disabled={!onUpload || busy}
            onClick={() => input.current?.click()}
            className={cn(
              'mt-1.5 flex w-full items-center justify-center gap-2 rounded-md border border-dashed border-border px-3 py-3 text-sm text-muted-foreground transition-colors hover:border-primary hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60',
            )}
          >
            {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : onUpload ? <Upload className="h-4 w-4" /> : <Paperclip className="h-4 w-4" />}
            {busy ? 'Uploading…' : onUpload ? 'Choose file' : 'File upload (disabled in preview)'}
          </button>
        </>
      ) : null}
      <p className="text-xs text-muted-foreground mt-1">
        {max > 1 ? `Up to ${max} files, ` : ''}max {maxMb} MB each
        {field.accept?.length ? ` · ${field.accept.join(', ')}` : ''}
      </p>
      {help}
      {uploadError ? <p className="text-xs text-destructive mt-1">{uploadError}</p> : null}
      {error ? <p className="text-xs text-destructive mt-1">{error}</p> : null}
    </div>
  );
}
