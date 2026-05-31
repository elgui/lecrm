-- leCRM demo seed — workspace "bistrot-halles" (Le Bistrot des Halles)
-- A French restaurant running a private-events / corporate-catering pipeline.
-- Idempotent: safe to run multiple times (ON CONFLICT DO NOTHING).
-- Usage: psql -v schema=workspace_xxx -f demo-bistrot-halles.sql
-- ⚠️  Pin search_path to the target workspace schema; everything below is unqualified.
-- UUIDs are reused across workspace schemas on purpose: schemas are isolated,
-- so identical IDs never collide and authoring stays simple. Only the data differs.

\set ON_ERROR_STOP on
SET search_path TO :'schema';

-- ============================================================
-- COMPANIES (4 clients qui privatisent / commandent du traiteur)
-- ============================================================
INSERT INTO companies (id, name, domain, industry, size, owner_id) VALUES
  ('c0000000-0000-4000-8000-0000000000a1', 'Cabinet Aubert & Associés', 'aubert-avocats.fr', 'Services juridiques', '11-50',    '00000000-0000-4000-8000-0000000000a1'),
  ('c0000000-0000-4000-8000-0000000000a2', 'Studio Lumière Photo',      'studiolumiere.fr',  'Médias',              '1-10',     '00000000-0000-4000-8000-0000000000a1'),
  ('c0000000-0000-4000-8000-0000000000a3', 'Groupe Pharma Rhône',       'pharma-rhone.fr',   'Pharmaceutique',      '201-1000', '00000000-0000-4000-8000-0000000000a1'),
  ('c0000000-0000-4000-8000-0000000000a4', 'Mairie de Saint-Genis',     'saint-genis.fr',    'Secteur public',      '51-200',   '00000000-0000-4000-8000-0000000000a1')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- CONTACTS (10 : organisateurs d'événements + 2 particuliers)
-- ============================================================
INSERT INTO contacts (id, first_name, last_name, email, phone, company_id, owner_id) VALUES
  ('11110000-0000-4000-8000-000000000001', 'Hélène',   'Aubert',    'helene@aubert-avocats.fr',   '+33 6 21 00 00 01', 'c0000000-0000-4000-8000-0000000000a1', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000002', 'Marc',     'Dubois',    'marc@studiolumiere.fr',      '+33 6 21 00 00 02', 'c0000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000003', 'Léa',      'Fontaine',  'lea@studiolumiere.fr',       '+33 6 21 00 00 03', 'c0000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000004', 'Nicolas',  'Reynaud',   'n.reynaud@pharma-rhone.fr',  '+33 6 21 00 00 04', 'c0000000-0000-4000-8000-0000000000a3', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000005', 'Sandrine', 'Colin',     's.colin@pharma-rhone.fr',    '+33 6 21 00 00 05', 'c0000000-0000-4000-8000-0000000000a3', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000006', 'Olivier',  'Mercier',   'o.mercier@pharma-rhone.fr',  '+33 6 21 00 00 06', 'c0000000-0000-4000-8000-0000000000a3', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000007', 'Christine','Vidal',     'c.vidal@saint-genis.fr',     '+33 4 72 00 00 07', 'c0000000-0000-4000-8000-0000000000a4', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000008', 'Franck',   'Leroy',     'f.leroy@saint-genis.fr',     '+33 4 72 00 00 08', 'c0000000-0000-4000-8000-0000000000a4', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000009', 'Émilie',   'Garnier',   'emilie.garnier@gmail.com',   '+33 6 21 00 55 01', NULL,                                   '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000010', 'Romain',   'Faure',     'romain.faure@outlook.fr',    '+33 6 21 00 55 02', NULL,                                   '00000000-0000-4000-8000-0000000000a1')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- DEALS (6 réservations d'événements sur les 5 étapes)
-- ============================================================
INSERT INTO deals (id, title, amount, currency, stage_id, contact_id, company_id, owner_id, expected_close_date, closed_at) VALUES
  ('dea10000-0000-4000-8000-000000000001', 'Cocktail de rentrée (50 pers.)',     4200,  'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Découverte'),         '11110000-0000-4000-8000-000000000001', 'c0000000-0000-4000-8000-0000000000a1', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 35, NULL),
  ('dea10000-0000-4000-8000-000000000002', 'Séminaire annuel (60 couverts)',     12500, 'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Qualifié'),           '11110000-0000-4000-8000-000000000002', 'c0000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 28, NULL),
  ('dea10000-0000-4000-8000-000000000003', 'Soirée de gala Pharma Rhône',        47000, 'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Proposition envoyée'), '11110000-0000-4000-8000-000000000004', 'c0000000-0000-4000-8000-0000000000a3', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 55, NULL),
  ('dea10000-0000-4000-8000-000000000004', 'Privatisation mariage Garnier',      31000, 'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Négociation'),        '11110000-0000-4000-8000-000000000009', 'c0000000-0000-4000-8000-0000000000a1', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 18, NULL),
  ('dea10000-0000-4000-8000-000000000005', 'Repas du conseil municipal (gagné)', 5400,  'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Gagné / Perdu'),      '11110000-0000-4000-8000-000000000007', 'c0000000-0000-4000-8000-0000000000a4', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE - 4,  now()),
  ('dea10000-0000-4000-8000-000000000006', 'Anniversaire 40 ans (perdu)',        9000,  'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Gagné / Perdu'),      '11110000-0000-4000-8000-000000000010', NULL,                                   '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE - 12, now())
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- ACTIVITIES (append-only timeline; actor_type='human_api')
-- ============================================================
INSERT INTO activities (id, entity_type, entity_id, actor_type, actor_id, event_type, payload) VALUES
  ('ac710000-0000-4000-8000-000000000001', 'deal',    'dea10000-0000-4000-8000-000000000003', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'deal.stage_changed', '{"from":"Qualifié","to":"Proposition envoyée"}'::jsonb),
  ('ac710000-0000-4000-8000-000000000002', 'deal',    'dea10000-0000-4000-8000-000000000004', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'deal.stage_changed', '{"from":"Proposition envoyée","to":"Négociation"}'::jsonb),
  ('ac710000-0000-4000-8000-000000000003', 'contact', '11110000-0000-4000-8000-000000000001', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'contact.created',     '{}'::jsonb),
  ('ac710000-0000-4000-8000-000000000004', 'deal',    'dea10000-0000-4000-8000-000000000005', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'deal.won',            '{"amount":5400}'::jsonb)
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- NOTES (notes libres)
-- ============================================================
INSERT INTO notes (id, entity_type, entity_id, body, author_id) VALUES
  ('0e700000-0000-4000-8000-000000000001', 'deal',    'dea10000-0000-4000-8000-000000000004', 'Budget 31k validé côté famille. Acompte 30% à la signature. Relance lundi.', '00000000-0000-4000-8000-0000000000a1'),
  ('0e700000-0000-4000-8000-000000000002', 'deal',    'dea10000-0000-4000-8000-000000000003', 'Concurrent traiteur en lice. On se différencie sur la cave et le service.',  '00000000-0000-4000-8000-0000000000a1'),
  ('0e700000-0000-4000-8000-000000000003', 'contact', '11110000-0000-4000-8000-000000000002', 'Préfère un échange par email. Très réactif sur les devis.',                  '00000000-0000-4000-8000-0000000000a1')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- TASKS (suivis à faire ; certains en retard, un terminé)
-- entity_type NULL => tâche globale, visible dans l'onglet Tasks.
-- Les tâches liées à un enregistrement remplissent aussi le panneau
-- "Tasks" des fiches contact / deal correspondantes.
-- ============================================================
INSERT INTO tasks (id, title, description, entity_type, entity_id, assignee_id, due_date, completed_at) VALUES
  ('7a5c0000-0000-4000-8000-000000000001', 'Relancer la famille Garnier pour l''acompte',   'Acompte 30% à la signature.',          'deal',    'dea10000-0000-4000-8000-000000000004', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 1,  NULL),
  ('7a5c0000-0000-4000-8000-000000000002', 'Envoyer le menu dégustation à Nicolas Reynaud', 'Soirée de gala Pharma Rhône.',         'deal',    'dea10000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 4,  NULL),
  ('7a5c0000-0000-4000-8000-000000000003', 'Confirmer le nombre de couverts',               'Cocktail de rentrée — 50 pers.',       'deal',    'dea10000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE - 1,  NULL),
  ('7a5c0000-0000-4000-8000-000000000004', 'Rappeler Marc pour le séminaire annuel',        'Studio Lumière — 60 couverts.',        'contact', '11110000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 2,  NULL),
  ('7a5c0000-0000-4000-8000-000000000005', 'Préparer la commande traiteur de la semaine',   NULL,                                   NULL,      NULL,                                   '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 3,  NULL),
  ('7a5c0000-0000-4000-8000-000000000006', 'Encaisser le solde — repas conseil municipal',  'Mairie de Saint-Genis.',               'deal',    'dea10000-0000-4000-8000-000000000005', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE - 2,  now() - INTERVAL '1 day')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- CUSTOM PROPERTY DEFINITIONS (thème événementiel)
-- Deduped on UNIQUE (parent_type, property_key). Value types match
-- metadata.validateValue: enum/string -> JSON string, number -> JSON number.
-- ============================================================
INSERT INTO custom_property_definitions (parent_type, property_key, property_type, allowed_values, required) VALUES
  ('deal',    'source_du_lead',  'enum',   '["Site web","Recommandation","Salon","LinkedIn"]'::jsonb,              false),
  ('deal',    'probabilite',     'number', NULL,                                                                   false),
  ('deal',    'canal_signature', 'string', NULL,                                                                   false),
  ('deal',    'type_evenement',  'enum',   '["Séminaire","Mariage","Cocktail","Repas d''affaires","Gala"]'::jsonb, false),
  ('contact', 'fonction',        'string', NULL,                                                                   false),
  ('contact', 'canal_prefere',   'enum',   '["Email","Téléphone","WhatsApp"]'::jsonb,                              false)
ON CONFLICT (parent_type, property_key) DO NOTHING;

-- ============================================================
-- CUSTOM PROPERTY VALUES (objects, object_type='custom_properties')
-- ============================================================
INSERT INTO objects (id, object_type, parent_type, parent_id, data) VALUES
  ('cf000000-0000-4000-8000-00000000d001', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000001', '{"source_du_lead":"Site web","probabilite":40,"canal_signature":"Email","type_evenement":"Cocktail"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000d002', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000002', '{"source_du_lead":"Recommandation","probabilite":60,"canal_signature":"En personne","type_evenement":"Séminaire"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000d003', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000003', '{"source_du_lead":"Salon","probabilite":75,"canal_signature":"Visio","type_evenement":"Gala"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000d004', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000004', '{"source_du_lead":"Recommandation","probabilite":85,"canal_signature":"En personne","type_evenement":"Mariage"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000d005', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000005', '{"source_du_lead":"Site web","probabilite":100,"canal_signature":"Email","type_evenement":"Repas d''affaires"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000d006', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000006', '{"source_du_lead":"Salon","probabilite":0,"canal_signature":"Téléphone","type_evenement":"Mariage"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000c001', 'custom_properties', 'contact', '11110000-0000-4000-8000-000000000001', '{"fonction":"Office manager","canal_prefere":"Email"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000c004', 'custom_properties', 'contact', '11110000-0000-4000-8000-000000000004', '{"fonction":"Directeur communication","canal_prefere":"Téléphone"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000c007', 'custom_properties', 'contact', '11110000-0000-4000-8000-000000000007', '{"fonction":"Chargée des affaires culturelles","canal_prefere":"Email"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000c009', 'custom_properties', 'contact', '11110000-0000-4000-8000-000000000009', '{"fonction":"Particulier","canal_prefere":"WhatsApp"}'::jsonb)
ON CONFLICT (id) DO NOTHING;

SELECT
    (SELECT count(*) FROM companies)  AS companies,
    (SELECT count(*) FROM contacts)   AS contacts,
    (SELECT count(*) FROM deals)      AS deals,
    (SELECT count(*) FROM tasks)      AS tasks;
