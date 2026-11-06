import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from '@tanstack/react-router';

import '@fontsource-variable/inter';
import '@fontsource-variable/montserrat';
import '../styles.css';

import { installAuth } from '@/auth/installAuth';
import { BrandProvider } from '@/brand/BrandProvider';
import { queryClient } from '@/lib/queryClient';
import { router } from '@/submit/router';

// Public submission site. Built from the same codebase as the staff app
// (RALLY_APP=submit) but signs in against the attendee realm.
installAuth();

const rootEl = document.getElementById('root');
if (!rootEl) {
  throw new Error('#root element missing from index.html');
}

createRoot(rootEl).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrandProvider productName={(import.meta.env.VITE_PRODUCT_NAME as string | undefined) || 'Event submissions'}>
        <RouterProvider router={router} />
      </BrandProvider>
    </QueryClientProvider>
  </StrictMode>,
);
