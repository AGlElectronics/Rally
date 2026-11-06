import { FileText } from 'lucide-react';
import { formatBytes, isEmptyAnswer, type Answers, type FormField } from '@/lib/forms';
import type { UploadedFile } from '@/lib/upload';

/** Read-only answers, in form order; answers to removed questions come last. */
export function AnswersView({
  fields,
  answers,
  files,
  onOpenFile,
}: {
  fields: FormField[];
  answers: Answers;
  files: UploadedFile[];
  onOpenFile?: (fileId: string) => void;
}) {
  const known = new Set(fields.map((f) => f.id));
  const orphans = Object.keys(answers).filter((k) => !known.has(k));
  return (
    <dl className="space-y-4">
      {fields.map((f) => (
        <div key={f.id}>
          <dt className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {f.label}
            {f.archived ? ' (removed question)' : ''}
          </dt>
          <dd className="mt-1 text-sm whitespace-pre-wrap break-words">
            <Value field={f} value={answers[f.id]} files={files} onOpenFile={onOpenFile} />
          </dd>
        </div>
      ))}
      {orphans.map((k) => (
        <div key={k}>
          <dt className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{k} (removed question)</dt>
          <dd className="mt-1 text-sm whitespace-pre-wrap break-words">{String(answers[k])}</dd>
        </div>
      ))}
    </dl>
  );
}

function Value({
  field,
  value,
  files,
  onOpenFile,
}: {
  field: FormField;
  value: Answers[string];
  files: UploadedFile[];
  onOpenFile?: (fileId: string) => void;
}) {
  if (isEmptyAnswer(value)) return <span className="text-muted-foreground">—</span>;
  switch (field.type) {
    case 'checkbox':
      return <>Yes</>;
    case 'multi_choice':
      return <>{(value as string[]).join(', ')}</>;
    case 'duration':
      return <>{String(value)} minutes</>;
    case 'datetime':
      return <>{new Date(String(value)).toLocaleString()}</>;
    case 'url':
      return (
        <a href={String(value)} target="_blank" rel="noreferrer noopener" className="text-primary underline">
          {String(value)}
        </a>
      );
    case 'file':
      return (
        <ul className="space-y-1">
          {(value as string[]).map((id) => {
            const f = files.find((x) => x.id === id);
            return (
              <li key={id}>
                <button
                  type="button"
                  className="inline-flex items-center gap-1.5 text-primary hover:underline"
                  onClick={() => onOpenFile?.(id)}
                >
                  <FileText className="h-4 w-4" />
                  {f?.filename ?? 'File'}
                  {f ? <span className="text-xs text-muted-foreground">({formatBytes(f.size_bytes)})</span> : null}
                </button>
              </li>
            );
          })}
        </ul>
      );
    default:
      return <>{String(value)}</>;
  }
}
