export interface User {
  id: string;
  email: string;
  name: string;
  picture?: string;
  workspace_id: string;
  workspace_slug: string;
}

export type Role = 'member' | 'admin' | 'owner' | 'none';

export interface Permissions {
  can_read: boolean;
  can_write: boolean;
  can_manage_members: boolean;
  can_manage_tokens: boolean;
  can_delete_workspace: boolean;
}

export interface Me {
  user_id: string;
  role: Role;
  actor_type: string;
  permissions: Permissions;
}

export interface Member {
  user_id: string;
  email: string | null;
  display_name: string | null;
  role: Role;
  invited_at: string;
  joined_at: string | null;
  pending: boolean;
}

export interface Contact {
  id: string;
  first_name: string;
  last_name: string;
  email: string | null;
  phone: string | null;
  company_id: string | null;
  owner_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface Company {
  id: string;
  name: string;
  domain: string | null;
  industry: string | null;
  size: string | null;
  owner_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface Deal {
  id: string;
  title: string;
  amount: number | null;
  currency: string | null;
  stage_id: string | null;
  contact_id: string | null;
  company_id: string | null;
  owner_id: string | null;
  expected_close_date: string | null;
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
