-- deploy/seed/demo.sql — populate a demo workspace so the CRM is not an
-- empty shell (staging tasket: ~10 contacts, 3-4 companies, 5-6 deals across
-- pipeline stages, a few activities/notes).
--
-- IDEMPOTENT: every row uses a fixed UUID + ON CONFLICT (id) DO NOTHING, so
-- re-running is a no-op. Safe to ship and re-apply.
--
-- SCHEMA-AGNOSTIC: the target workspace schema is passed as the psql var
-- `schema`; this file pins search_path to it. The workspace must already be
-- provisioned with the `gbconsult-default` template (so pipeline_stages
-- exists and is seeded with Discovery/Qualified/Proposal Sent/Negotiation/
-- Closed-Won/Lost) and lecrm_api must hold DML on it (migration 0017).
--
-- Run:
--   psql "$SUPERUSER_DSN" \
--     -v schema=workspace_<uuid-no-dashes> \
--     -f deploy/seed/demo.sql
--
-- (See deploy/README.md staging runbook for resolving <uuid> from
--  core.workspaces WHERE slug = 'demo'.)

\set ON_ERROR_STOP on

SET search_path TO :"schema";

-- A single fixed "demo owner" id stands in for the workspace's human user.
-- core.users rows are created on real OIDC login; owner_id/author_id here are
-- not FK-constrained (cross-schema FKs violate tenant isolation — ADR-001),
-- so a stable sentinel uuid is sufficient for a populated demo.
--   demo owner = 000...0a1

-- ---------------------------------------------------------------- companies
INSERT INTO companies (id, name, domain, industry, size, owner_id) VALUES
  ('c0000000-0000-4000-8000-0000000000a1', 'Boulangerie Lefèvre',      'lefevre-pains.fr',   'Food & Beverage',  '11-50',   '00000000-0000-4000-8000-0000000000a1'),
  ('c0000000-0000-4000-8000-0000000000a2', 'Studio Marceau Design',    'marceau.design',     'Design Agency',    '1-10',    '00000000-0000-4000-8000-0000000000a1'),
  ('c0000000-0000-4000-8000-0000000000a3', 'TransAlpes Logistique',    'transalpes.fr',      'Logistics',        '201-1000','00000000-0000-4000-8000-0000000000a1'),
  ('c0000000-0000-4000-8000-0000000000a4', 'Clinique du Parc',         'cliniqueduparc.fr',  'Healthcare',       '51-200',  '00000000-0000-4000-8000-0000000000a1')
ON CONFLICT (id) DO NOTHING;

-- ----------------------------------------------------------------- contacts
INSERT INTO contacts (id, first_name, last_name, email, phone, company_id, owner_id) VALUES
  ('11110000-0000-4000-8000-000000000001', 'Camille',  'Lefèvre',   'camille@lefevre-pains.fr',   '+33 4 72 00 11 01', 'c0000000-0000-4000-8000-0000000000a1', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000002', 'Hugo',     'Berthier',  'hugo.berthier@lefevre-pains.fr','+33 4 72 00 11 02', 'c0000000-0000-4000-8000-0000000000a1', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000003', 'Léa',      'Marceau',   'lea@marceau.design',         '+33 1 44 00 22 01', 'c0000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000004', 'Nicolas',  'Roux',      'nicolas@marceau.design',     '+33 1 44 00 22 02', 'c0000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000005', 'Sophie',   'Garnier',   's.garnier@transalpes.fr',    '+33 4 76 00 33 01', 'c0000000-0000-4000-8000-0000000000a3', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000006', 'Marc',     'Fontaine',  'm.fontaine@transalpes.fr',   '+33 4 76 00 33 02', 'c0000000-0000-4000-8000-0000000000a3', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000007', 'Inès',     'Dubois',    'ines.dubois@cliniqueduparc.fr','+33 4 78 00 44 01','c0000000-0000-4000-8000-0000000000a4', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000008', 'Thomas',   'Mercier',   't.mercier@cliniqueduparc.fr','+33 4 78 00 44 02', 'c0000000-0000-4000-8000-0000000000a4', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000009', 'Julie',    'Petit',     'julie.petit@gmail.com',      '+33 6 12 00 55 01', NULL,                                   '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000010', 'Antoine',  'Lambert',   'antoine.lambert@outlook.fr', '+33 6 12 00 55 02', NULL,                                   '00000000-0000-4000-8000-0000000000a1')
ON CONFLICT (id) DO NOTHING;

-- -------------------------------------------------------------------- deals
-- Spread across all five gbconsult-default stages. stage_id is resolved by
-- name from the seeded pipeline_stages table.
INSERT INTO deals (id, title, amount, currency, stage_id, contact_id, company_id, owner_id, expected_close_date, closed_at) VALUES
  ('dea10000-0000-4000-8000-000000000001', 'Site vitrine + click & collect',  8500.00,  'EUR',
     (SELECT id FROM pipeline_stages WHERE name = 'Discovery'),
     '11110000-0000-4000-8000-000000000001', 'c0000000-0000-4000-8000-0000000000a1', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 45, NULL),
  ('dea10000-0000-4000-8000-000000000002', 'Refonte identité de marque',      14000.00, 'EUR',
     (SELECT id FROM pipeline_stages WHERE name = 'Qualified'),
     '11110000-0000-4000-8000-000000000003', 'c0000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 30, NULL),
  ('dea10000-0000-4000-8000-000000000003', 'Optimisation tournées (TMS)',     52000.00, 'EUR',
     (SELECT id FROM pipeline_stages WHERE name = 'Proposal Sent'),
     '11110000-0000-4000-8000-000000000005', 'c0000000-0000-4000-8000-0000000000a3', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 20, NULL),
  ('dea10000-0000-4000-8000-000000000004', 'Portail patients & prise de RDV', 38000.00, 'EUR',
     (SELECT id FROM pipeline_stages WHERE name = 'Negotiation'),
     '11110000-0000-4000-8000-000000000007', 'c0000000-0000-4000-8000-0000000000a4', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 10, NULL),
  ('dea10000-0000-4000-8000-000000000005', 'Maintenance annuelle (renouvelé)', 6000.00, 'EUR',
     (SELECT id FROM pipeline_stages WHERE name = 'Closed-Won/Lost'),
     '11110000-0000-4000-8000-000000000003', 'c0000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE - 5, now() - INTERVAL '5 days'),
  ('dea10000-0000-4000-8000-000000000006', 'Audit logistique (perdu)',        21000.00, 'EUR',
     (SELECT id FROM pipeline_stages WHERE name = 'Closed-Won/Lost'),
     '11110000-0000-4000-8000-000000000006', 'c0000000-0000-4000-8000-0000000000a3', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE - 12, now() - INTERVAL '12 days')
ON CONFLICT (id) DO NOTHING;

-- --------------------------------------------------------------- activities
-- Append-only timeline entries. actor_type='human_api' (REST writes).
INSERT INTO activities (id, entity_type, entity_id, actor_type, actor_id, event_type, payload) VALUES
  ('ac710000-0000-4000-8000-000000000001', 'deal', 'dea10000-0000-4000-8000-000000000003', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'deal.stage_changed', '{"from":"Qualified","to":"Proposal Sent"}'::jsonb),
  ('ac710000-0000-4000-8000-000000000002', 'deal', 'dea10000-0000-4000-8000-000000000004', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'deal.stage_changed', '{"from":"Proposal Sent","to":"Negotiation"}'::jsonb),
  ('ac710000-0000-4000-8000-000000000003', 'contact', '11110000-0000-4000-8000-000000000001', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'contact.created', '{}'::jsonb),
  ('ac710000-0000-4000-8000-000000000004', 'deal', 'dea10000-0000-4000-8000-000000000005', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'deal.won', '{"amount":6000}'::jsonb)
ON CONFLICT (id) DO NOTHING;

-- -------------------------------------------------------------------- notes
INSERT INTO notes (id, entity_type, entity_id, body, author_id) VALUES
  ('0e700000-0000-4000-8000-000000000001', 'deal', 'dea10000-0000-4000-8000-000000000003', 'Appel découverte fait — 14 véhicules, besoin TMS + suivi temps réel. Décision Q3.', '00000000-0000-4000-8000-0000000000a1'),
  ('0e700000-0000-4000-8000-000000000002', 'deal', 'dea10000-0000-4000-8000-000000000004', 'Devis envoyé. Négociation sur le module de prise de RDV. Sponsor: Dr. Dubois.', '00000000-0000-4000-8000-0000000000a1'),
  ('0e700000-0000-4000-8000-000000000003', 'contact', '11110000-0000-4000-8000-000000000003', 'Préfère être contactée par email. Très réactive.', '00000000-0000-4000-8000-0000000000a1')
ON CONFLICT (id) DO NOTHING;

-- ------------------------------------------------------------------ summary
DO $$
DECLARE n_co INT; n_ct INT; n_de INT; n_ac INT; n_no INT;
BEGIN
  SELECT count(*) INTO n_co FROM companies;
  SELECT count(*) INTO n_ct FROM contacts;
  SELECT count(*) INTO n_de FROM deals;
  SELECT count(*) INTO n_ac FROM activities;
  SELECT count(*) INTO n_no FROM notes;
  RAISE NOTICE 'demo seed: % companies, % contacts, % deals, % activities, % notes',
    n_co, n_ct, n_de, n_ac, n_no;
END$$;
