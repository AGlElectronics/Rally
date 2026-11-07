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
import { runtimeConfig } from '@/runtime-config';

export function startApp(rootEl: HTMLElement): void {
  installAuth();

  createRoot(rootEl).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <BrandProvider productName={runtimeConfig().productName || 'Event submissions'}>
          <RouterProvider router={router} />
        </BrandProvider>
      </QueryClientProvider>
    </StrictMode>,
  );
}
