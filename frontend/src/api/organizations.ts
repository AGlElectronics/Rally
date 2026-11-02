import { apiRequest } from '@/api/client';

export type Organization = {
  id: string;
  slug: string;
  display_name: string;
  role?: string;
};

export type OrganizationList = {
  items: Organization[];
  total: number;
};

export function listOrganizations(signal?: AbortSignal): Promise<OrganizationList> {
  return apiRequest<OrganizationList>('/api/v1/organizations', { signal });
}

export function getOrganization(id: string, signal?: AbortSignal): Promise<Organization> {
  return apiRequest<Organization>(`/api/v1/organizations/${id}`, { signal });
}
