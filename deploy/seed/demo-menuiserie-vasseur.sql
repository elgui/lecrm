-- leCRM demo seed — workspace "menuiserie-vasseur" (Menuiserie Vasseur)
-- A French custom-carpentry workshop running a project/quote (chantiers) pipeline.
-- Idempotent: safe to run multiple times (ON CONFLICT DO NOTHING).
-- Usage: psql -v schema=workspace_xxx -f demo-menuiserie-vasseur.sql
-- ⚠️  Pin search_path to the target workspace schema; everything below is unqualified.
-- UUIDs are reused across workspace schemas on purpose: schemas are isolated,
-- so identical IDs never collide and authoring stays simple. Only the data differs.

\set ON_ERROR_STOP on
SET search_path TO :'schema';

-- ============================================================
-- COMPANIES (4 clients B2B : archi, promoteur, retail, hôtellerie)
-- ============================================================
INSERT INTO companies (id, name, domain, industry, size, owner_id) VALUES
  ('c0000000-0000-4000-8000-0000000000a1', 'Atelier d''Architecture Moreau', 'moreau-archi.fr',  'Architecture', '1-10',  '00000000-0000-4000-8000-0000000000a1'),
  ('c0000000-0000-4000-8000-0000000000a2', 'Promotion Cévennes Immobilier',  'cevennes-immo.fr', 'Immobilier',   '11-50', '00000000-0000-4000-8000-0000000000a1'),
  ('c0000000-0000-4000-8000-0000000000a3', 'Boutiques Lina Mode',            'lina-mode.fr',     'Commerce',     '11-50', '00000000-0000-4000-8000-0000000000a1'),
  ('c0000000-0000-4000-8000-0000000000a4', 'Hôtel Le Belvédère',             'hotel-belvedere.fr','Hôtellerie',  '51-200','00000000-0000-4000-8000-0000000000a1')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- CONTACTS (10 : prescripteurs B2B + 2 particuliers)
-- ============================================================
INSERT INTO contacts (id, first_name, last_name, email, phone, company_id, owner_id) VALUES
  ('11110000-0000-4000-8000-000000000001', 'Julien',     'Moreau',     'julien@moreau-archi.fr',     '+33 6 31 00 00 01', 'c0000000-0000-4000-8000-0000000000a1', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000002', 'Cécile',     'Brun',       'c.brun@cevennes-immo.fr',    '+33 6 31 00 00 02', 'c0000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000003', 'David',      'Roche',      'd.roche@cevennes-immo.fr',   '+33 6 31 00 00 03', 'c0000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000004', 'Lina',       'Costa',      'lina@lina-mode.fr',          '+33 6 31 00 00 04', 'c0000000-0000-4000-8000-0000000000a3', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000005', 'Patrick',    'Simon',      'p.simon@lina-mode.fr',       '+33 6 31 00 00 05', 'c0000000-0000-4000-8000-0000000000a3', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000006', 'Nathalie',   'Imbert',     'n.imbert@hotel-belvedere.fr','+33 4 66 00 00 06', 'c0000000-0000-4000-8000-0000000000a4', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000007', 'Guillaume',  'Astier',     'g.astier@hotel-belvedere.fr','+33 4 66 00 00 07', 'c0000000-0000-4000-8000-0000000000a4', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000008', 'Sébastien',  'Pons',       's.pons@moreau-archi.fr',     '+33 6 31 00 00 08', 'c0000000-0000-4000-8000-0000000000a1', '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000009', 'Vincent',    'Charpentier','vincent.charpentier@gmail.com','+33 6 31 00 55 01', NULL,                                 '00000000-0000-4000-8000-0000000000a1'),
  ('11110000-0000-4000-8000-000000000010', 'Aurélie',    'Delmas',     'aurelie.delmas@outlook.fr',  '+33 6 31 00 55 02', NULL,                                   '00000000-0000-4000-8000-0000000000a1')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- DEALS (6 chantiers / devis sur les 5 étapes)
-- ============================================================
INSERT INTO deals (id, title, amount, currency, stage_id, contact_id, company_id, owner_id, expected_close_date, closed_at) VALUES
  ('dea10000-0000-4000-8000-000000000001', 'Agencement vitrine boutique',         7800,  'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Découverte'),         '11110000-0000-4000-8000-000000000004', 'c0000000-0000-4000-8000-0000000000a3', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 40, NULL),
  ('dea10000-0000-4000-8000-000000000002', 'Escalier chêne sur-mesure',           16500, 'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Qualifié'),           '11110000-0000-4000-8000-000000000009', NULL,                                   '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 25, NULL),
  ('dea10000-0000-4000-8000-000000000003', 'Menuiseries ext. résidence (32 lots)',58000, 'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Proposition envoyée'), '11110000-0000-4000-8000-000000000002', 'c0000000-0000-4000-8000-0000000000a2', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 70, NULL),
  ('dea10000-0000-4000-8000-000000000004', 'Mobilier chambres hôtel (18 ch.)',    41000, 'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Négociation'),        '11110000-0000-4000-8000-000000000006', 'c0000000-0000-4000-8000-0000000000a4', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 22, NULL),
  ('dea10000-0000-4000-8000-000000000005', 'Pose parquet showroom (gagné)',       9200,  'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Gagné / Perdu'),      '11110000-0000-4000-8000-000000000001', 'c0000000-0000-4000-8000-0000000000a1', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE - 6,  now()),
  ('dea10000-0000-4000-8000-000000000006', 'Bardage bois façade (perdu)',         23000, 'EUR', (SELECT id FROM pipeline_stages WHERE name = 'Gagné / Perdu'),      '11110000-0000-4000-8000-000000000010', NULL,                                   '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE - 14, now())
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- ACTIVITIES (append-only timeline; actor_type='human_api')
-- ============================================================
INSERT INTO activities (id, entity_type, entity_id, actor_type, actor_id, event_type, payload) VALUES
  ('ac710000-0000-4000-8000-000000000001', 'deal',    'dea10000-0000-4000-8000-000000000003', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'deal.stage_changed', '{"from":"Qualifié","to":"Proposition envoyée"}'::jsonb),
  ('ac710000-0000-4000-8000-000000000002', 'deal',    'dea10000-0000-4000-8000-000000000004', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'deal.stage_changed', '{"from":"Proposition envoyée","to":"Négociation"}'::jsonb),
  ('ac710000-0000-4000-8000-000000000003', 'contact', '11110000-0000-4000-8000-000000000001', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'contact.created',     '{}'::jsonb),
  ('ac710000-0000-4000-8000-000000000004', 'deal',    'dea10000-0000-4000-8000-000000000005', 'human_api', '00000000-0000-4000-8000-0000000000a1', 'deal.won',            '{"amount":9200}'::jsonb)
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- NOTES (notes libres)
-- ============================================================
INSERT INTO notes (id, entity_type, entity_id, body, author_id) VALUES
  ('0e700000-0000-4000-8000-000000000001', 'deal',    'dea10000-0000-4000-8000-000000000004', 'Hôtel veut un prototype chambre avant commande des 18. Échantillons prévus.', '00000000-0000-4000-8000-0000000000a1'),
  ('0e700000-0000-4000-8000-000000000002', 'deal',    'dea10000-0000-4000-8000-000000000003', 'Appel d''offres : 3 menuisiers consultés. Atout = atelier intégré, délais.',  '00000000-0000-4000-8000-0000000000a1'),
  ('0e700000-0000-4000-8000-000000000003', 'contact', '11110000-0000-4000-8000-000000000001', 'Architecte prescripteur clé. Privilégier les échanges par email + plans.',   '00000000-0000-4000-8000-0000000000a1')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- TASKS (suivis à faire ; certains en retard, un terminé)
-- entity_type NULL => tâche globale, visible dans l'onglet Tasks.
-- Les tâches liées à un enregistrement remplissent aussi le panneau
-- "Tasks" des fiches contact / deal correspondantes.
-- ============================================================
INSERT INTO tasks (id, title, description, entity_type, entity_id, assignee_id, due_date, completed_at) VALUES
  ('7a5c0000-0000-4000-8000-000000000001', 'Livrer le prototype chambre à l''Hôtel Belvédère', 'Validation avant commande des 18 ch.', 'deal',    'dea10000-0000-4000-8000-000000000004', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 2,  NULL),
  ('7a5c0000-0000-4000-8000-000000000002', 'Finaliser le chiffrage des 32 lots',               'Menuiseries ext. résidence Cévennes.',  'deal',    'dea10000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 5,  NULL),
  ('7a5c0000-0000-4000-8000-000000000003', 'Prendre les côtes en boutique',                    'Agencement vitrine Lina Mode.',         'deal',    'dea10000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE - 2,  NULL),
  ('7a5c0000-0000-4000-8000-000000000004', 'Envoyer les plans à Julien Moreau',                'Architecte prescripteur clé.',          'contact', '11110000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 1,  NULL),
  ('7a5c0000-0000-4000-8000-000000000005', 'Planifier les poses de la semaine',                NULL,                                    NULL,      NULL,                                   '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE + 3,  NULL),
  ('7a5c0000-0000-4000-8000-000000000006', 'Établir la facture du parquet showroom',           'Pose parquet showroom — gagné.',        'deal',    'dea10000-0000-4000-8000-000000000005', '00000000-0000-4000-8000-0000000000a1', CURRENT_DATE - 4,  now() - INTERVAL '1 day')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- CUSTOM PROPERTY DEFINITIONS (thème menuiserie)
-- Deduped on UNIQUE (parent_type, property_key). Value types match
-- metadata.validateValue: enum/string -> JSON string, number -> JSON number.
-- ============================================================
INSERT INTO custom_property_definitions (parent_type, property_key, property_type, allowed_values, required) VALUES
  ('deal',    'source_du_lead',  'enum',   '["Site web","Recommandation","Salon","LinkedIn"]'::jsonb,                  false),
  ('deal',    'probabilite',     'number', NULL,                                                                       false),
  ('deal',    'canal_signature', 'string', NULL,                                                                       false),
  ('deal',    'type_ouvrage',    'enum',   '["Agencement","Escalier","Menuiserie ext.","Mobilier","Parquet"]'::jsonb, false),
  ('contact', 'fonction',        'string', NULL,                                                                       false),
  ('contact', 'canal_prefere',   'enum',   '["Email","Téléphone","WhatsApp"]'::jsonb,                                 false)
ON CONFLICT (parent_type, property_key) DO NOTHING;

-- ============================================================
-- CUSTOM PROPERTY VALUES (objects, object_type='custom_properties')
-- ============================================================
INSERT INTO objects (id, object_type, parent_type, parent_id, data) VALUES
  ('cf000000-0000-4000-8000-00000000d001', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000001', '{"source_du_lead":"Recommandation","probabilite":45,"canal_signature":"En personne","type_ouvrage":"Agencement"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000d002', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000002', '{"source_du_lead":"Site web","probabilite":65,"canal_signature":"En personne","type_ouvrage":"Escalier"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000d003', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000003', '{"source_du_lead":"Salon","probabilite":55,"canal_signature":"Email","type_ouvrage":"Menuiserie ext."}'::jsonb),
  ('cf000000-0000-4000-8000-00000000d004', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000004', '{"source_du_lead":"Recommandation","probabilite":80,"canal_signature":"Visio","type_ouvrage":"Mobilier"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000d005', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000005', '{"source_du_lead":"Recommandation","probabilite":100,"canal_signature":"En personne","type_ouvrage":"Parquet"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000d006', 'custom_properties', 'deal',    'dea10000-0000-4000-8000-000000000006', '{"source_du_lead":"Salon","probabilite":0,"canal_signature":"Téléphone","type_ouvrage":"Menuiserie ext."}'::jsonb),
  ('cf000000-0000-4000-8000-00000000c001', 'custom_properties', 'contact', '11110000-0000-4000-8000-000000000001', '{"fonction":"Architecte DPLG","canal_prefere":"Email"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000c002', 'custom_properties', 'contact', '11110000-0000-4000-8000-000000000002', '{"fonction":"Responsable programmes","canal_prefere":"Téléphone"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000c006', 'custom_properties', 'contact', '11110000-0000-4000-8000-000000000006', '{"fonction":"Directrice d''exploitation","canal_prefere":"Email"}'::jsonb),
  ('cf000000-0000-4000-8000-00000000c009', 'custom_properties', 'contact', '11110000-0000-4000-8000-000000000009', '{"fonction":"Particulier","canal_prefere":"WhatsApp"}'::jsonb)
ON CONFLICT (id) DO NOTHING;

SELECT
    (SELECT count(*) FROM companies)  AS companies,
    (SELECT count(*) FROM contacts)   AS contacts,
    (SELECT count(*) FROM deals)      AS deals,
    (SELECT count(*) FROM tasks)      AS tasks;
