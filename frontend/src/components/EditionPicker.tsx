import type { Edition } from '@/api/cfp';
import { selectClasses } from '@/components/ui/Pill';

export function EditionPicker({
  editions,
  edition,
  onChange,
}: {
  editions: Edition[];
  edition: Edition | undefined;
  onChange: (id: string) => void;
}) {
  if (editions.length <= 1) return null;
  return (
    <select aria-label="Edition" className={selectClasses} value={edition?.id ?? ''} onChange={(e) => onChange(e.target.value)}>
      {editions.map((ed) => (
        <option key={ed.id} value={ed.id}>
          {ed.title}
        </option>
      ))}
    </select>
  );
}
