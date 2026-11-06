import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { listEditions, type Edition } from '@/api/cfp';

const KEY = 'rally.edition';

/** The edition the staff screens work on; remembered across pages and visits. */
export function useEdition(): {
  editions: Edition[];
  edition: Edition | undefined;
  setEditionId: (id: string) => void;
  isLoading: boolean;
  error: unknown;
} {
  const q = useQuery({ queryKey: ['editions'], queryFn: listEditions, staleTime: 60_000 });
  const [picked, setPicked] = useState(() => localStorage.getItem(KEY) ?? '');
  const editions = q.data?.items ?? [];
  const edition = editions.find((e) => e.id === picked) ?? editions[0];
  return {
    editions,
    edition,
    setEditionId: (id) => {
      localStorage.setItem(KEY, id);
      setPicked(id);
    },
    isLoading: q.isLoading,
    error: q.error,
  };
}
