export interface User {
  id: string;
  email: string;
  name: string;
  picture?: string;
  workspace_id: string;
  workspace_slug: string;
}

export interface Contact {
  id: string;
  workspace_id: string;
  first_name: string;
  last_name: string;
  email: string | null;
  phone: string | null;
  company_id: string | null;
  company_name?: string;
  created_at: string;
  updated_at: string;
}

export interface Company {
  id: string;
  workspace_id: string;
  name: string;
  domain: string | null;
  created_at: string;
  updated_at: string;
}

export interface Deal {
  id: string;
  workspace_id: string;
  title: string;
  stage: string;
  amount: number | null;
  currency: string;
  contact_id: string | null;
  company_id: string | null;
  closed_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface CustomProperty {
  key: string;
  value: unknown;
  type: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  next_cursor: string | null;
  has_more: boolean;
}
