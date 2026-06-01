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

export type PropertyType = 'string' | 'number' | 'boolean' | 'enum' | 'date' | 'json';

export interface PropertyDefinition {
  id: string;
  parent_type: 'contact' | 'deal';
  property_key: string;
  property_type: PropertyType;
  allowed_values?: string[];
  required: boolean;
  // Optional human-readable label set by an admin. When absent, the UI falls
  // back to a prettified form of property_key (see customFieldLabel). Kept
  // optional so older API responses without the field still type-check.
  display_name?: string | null;
}

export type EntityType = 'contact' | 'company' | 'deal';

export interface Note {
  id: string;
  entity_type: EntityType;
  entity_id: string;
  body: string;
  author_id: string;
  created_at: string;
  updated_at: string;
}

export interface Task {
  id: string;
  title: string;
  description: string | null;
  entity_type: EntityType | null;
  entity_id: string | null;
  assignee_id: string | null;
  due_date: string | null;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  next_cursor: string | null;
  has_more: boolean;
}

export interface AccessibleWorkspace {
  slug: string;
  role: string;
  url: string;
}

export interface DedupContactRecord {
  id: string;
  first_name: string;
  last_name: string;
  email: string | null;
  phone: string | null;
  company_id: string | null;
  created_at: string;
}

export interface DedupCompanyRecord {
  id: string;
  name: string;
  domain: string | null;
  industry: string | null;
  created_at: string;
}

export interface DedupContactPair {
  a: DedupContactRecord;
  b: DedupContactRecord;
  reason: 'exact_email' | 'similar_name';
  score: number;
}

export interface DedupCompanyPair {
  a: DedupCompanyRecord;
  b: DedupCompanyRecord;
  reason: 'exact_domain' | 'similar_name';
  score: number;
}

export interface MergeResult {
  survivor_id: string;
  loser_id: string;
  merged: boolean;
}
