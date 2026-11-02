import { apiRequest } from '@/api/client';

export type ApiBrand = {
  revision: number;
  source: 'standalone' | 'den';
  productName: string;
  organizationName?: string;
  loginSubtitle?: string;
  loginButtonLabel?: string;
  logoUrl?: string;
  themeColor?: string;
  colors?: Record<string, string>;
  cssVariables?: Record<string, string>;
};

/** Unauthenticated — called at app boot before the login screen renders. */
export function getBrand(signal?: AbortSignal): Promise<ApiBrand> {
  return apiRequest<ApiBrand>('/api/v1/brand', { signal });
}
