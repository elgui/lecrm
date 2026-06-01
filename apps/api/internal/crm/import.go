package crm

// CSV import — Integrator-gap tasket (20260601-110828-736b).
//
// The companion to CSV export (export.go). Onboarding the ICP (spreadsheet /
// Notion / Airtable / abandoned-HubSpot-Free crowd, docs/ICP-ARCHETYPE.md
// filter #4) starts with getting existing data in. This is the *basic* CSV
// import — contacts, companies and deals, one entity type per import — not a
// deep HubSpot API migrator (kept deliberately within filter #4's scope).
//
// Flow (three POSTs, all under /v1/import/{entity}/…):
//
//	analyze → parse the CSV header + a few sample rows, return the mappable
//	          core fields and the workspace's custom-property definitions plus
//	          a best-effort suggested mapping. No DB write.
//	preview → apply a column mapping + dedup policy, classify every row as
//	          create / update / skip / error against the *current* workspace
//	          data, return counts and per-row outcomes. Read-only: nothing is
//	          written until the user confirms.
//	commit  → same classification, but inside one write transaction: rows are
//	          inserted/updated, custom properties merged, a per-entity activity
//	          emitted, and a single batch audit event written to
//	          core.audit_log (ADR-007). Errored rows are collected into a
//	          downloadable report, never abort the batch; a fatal DB error
//	          rolls the whole transaction back (fail-closed, ADR-009 §7.2).
//
// Every read/write runs inside the caller's workspace schema (search_path set
// by readTx/writeTx), so an import can only ever touch the caller's own tenant
// — the sovereignty guarantee (ADR-009 §1) holds for writes exactly as it does
// for the export reads.

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gbconsult/lecrm/apps/api/capability"
	"github.com/gbconsult/lecrm/apps/api/internal/sqlcgen"
)

// maxImportBodySize bounds an import request body. CSV import payloads are
// larger than the 1 MiB CRUD limit (a full contact export round-trips back in
// here), so we allow 16 MiB — enough for tens of thousands of onboarding rows
// while still capping a hostile upload.
const maxImportBodySize int64 = 16 << 20

// maxSampleRows is how many data rows analyze echoes back for the mapping UI.
const maxSampleRows = 5

// dedupe policies — what to do when an incoming row matches an existing record.
const (
	dedupeUpdate = "update" // overlay the mapped fields onto the existing record
	dedupeSkip   = "skip"   // leave the existing record untouched, count as skipped
	dedupeCreate = "create" // ignore the match, always insert a new record
)

// row outcome actions.
const (
	actionCreate = "create"
	actionUpdate = "update"
	actionSkip   = "skip"
	actionError  = "error"
)

// coreFieldSpec describes one mappable built-in column of an entity.
type coreFieldSpec struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

// importEntitySpec is the static description of an importable entity type.
type importEntitySpec struct {
	entity     string // singular: contact | company | deal
	plural     string // contacts | companies | deals (the URL segment)
	parentType string // custom-property parent_type ("" when none)
	coreFields []coreFieldSpec
	dedupe     bool // contacts/companies dedup; deals never do
}

func contactSpec() importEntitySpec {
	return importEntitySpec{
		entity: "contact", plural: "contacts", parentType: entityTypeContact, dedupe: true,
		coreFields: []coreFieldSpec{
			{Key: "first_name", Label: "Prénom", Required: true},
			{Key: "last_name", Label: "Nom", Required: true},
			{Key: "email", Label: "E-mail", Required: false},
			{Key: "phone", Label: "Téléphone", Required: false},
		},
	}
}

func companySpec() importEntitySpec {
	return importEntitySpec{
		entity: "company", plural: "companies", parentType: "", dedupe: true,
		coreFields: []coreFieldSpec{
			{Key: "name", Label: "Nom", Required: true},
			{Key: "domain", Label: "Domaine", Required: false},
			{Key: "industry", Label: "Secteur", Required: false},
			{Key: "size", Label: "Taille", Required: false},
		},
	}
}

func dealSpec() importEntitySpec {
	return importEntitySpec{
		entity: "deal", plural: "deals", parentType: entityTypeDeal, dedupe: false,
		coreFields: []coreFieldSpec{
			{Key: "title", Label: "Titre", Required: true},
			{Key: "amount", Label: "Montant", Required: false},
			{Key: "currency", Label: "Devise", Required: false},
			{Key: "stage", Label: "Étape", Required: false},
			{Key: "expected_close_date", Label: "Clôture prévue", Required: false},
		},
	}
}

// specForParam maps the {entity} URL segment to its spec. The plural form is
// what the export endpoints and the React routes already use.
func specForParam(param string) (importEntitySpec, bool) {
	switch param {
	case "contacts":
		return contactSpec(), true
	case "companies":
		return companySpec(), true
	case "deals":
		return dealSpec(), true
	default:
		return importEntitySpec{}, false
	}
}

// importDef is a custom-property definition surfaced to the mapping UI.
type importDef struct {
	Key          string   `json:"key"`           // property_key
	Label        string   `json:"label"`         // display name or prettified key
	PropertyType string   `json:"property_type"` // string|number|boolean|enum|date|json
	Allowed      []string `json:"allowed_values,omitempty"`
}

// --- wire types ---

type importMappingReq struct {
	CSV     string            `json:"csv"`
	Mapping map[string]string `json:"mapping"` // csv column header -> target ("first_name" | "cf_<key>")
	Dedupe  string            `json:"dedupe"`  // update | skip | create
}

type analyzeResp struct {
	Columns          []string          `json:"columns"`
	SampleRows       [][]string        `json:"sample_rows"`
	RowCount         int               `json:"row_count"`
	CoreFields       []coreFieldSpec   `json:"core_fields"`
	CustomFields     []importDef       `json:"custom_fields"`
	SuggestedMapping map[string]string `json:"suggested_mapping"`
}

type importSummary struct {
	Total   int `json:"total"`
	Create  int `json:"create"`
	Update  int `json:"update"`
	Skip    int `json:"skip"`
	Error   int `json:"error"`
}

type rowOutcome struct {
	Line   int    `json:"line"`   // 1-based CSV line number (header is line 1)
	Action string `json:"action"` // create|update|skip|error
	Reason string `json:"reason,omitempty"`
	Label  string `json:"label,omitempty"` // human hint for the row (name/email/title)
}

type previewResp struct {
	Summary importSummary `json:"summary"`
	Rows    []rowOutcome  `json:"rows"`
}

type commitResp struct {
	Summary        importSummary `json:"summary"`
	ErrorReportCSV string        `json:"error_report_csv"` // downloadable report of skipped/errored rows
	AuditEvent     string        `json:"audit_event"`
}

// --- route registration is in handlers.go RegisterRoutes ---

// ImportAnalyze parses the uploaded CSV's header + sample rows and returns the
// mappable fields (core + custom-property definitions) with a suggested
// mapping. No data is written.
func (h *Handler) ImportAnalyze(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	spec, ok := specForParam(chi.URLParam(r, "entity"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown import entity")
		return
	}
	var body importMappingReq
	if !decodeImportBody(w, r, &body) {
		return
	}
	header, records, err := parseCSVText(body.CSV)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	var defs []importDef
	if spec.parentType != "" {
		if e := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
			var le error
			defs, le = loadImportDefs(r.Context(), tx, spec.parentType)
			return le
		}); e != nil {
			h.Logger.ErrorContext(r.Context(), "import analyze defs", "err", e)
			writeErr(w, http.StatusInternalServerError, "load definitions failed")
			return
		}
	}

	sample := records
	if len(sample) > maxSampleRows {
		sample = sample[:maxSampleRows]
	}

	writeJSON(w, http.StatusOK, analyzeResp{
		Columns:          header,
		SampleRows:       sample,
		RowCount:         len(records),
		CoreFields:       spec.coreFields,
		CustomFields:     defs,
		SuggestedMapping: suggestMapping(header, spec, defs),
	})
}

// ImportPreview classifies every CSV row (create/update/skip/error) against
// current workspace data without writing anything (dry run).
func (h *Handler) ImportPreview(w http.ResponseWriter, r *http.Request) {
	h.importRun(w, r, false)
}

// ImportCommit performs the import inside one write transaction and returns a
// summary plus a downloadable error report.
func (h *Handler) ImportCommit(w http.ResponseWriter, r *http.Request) {
	h.importRun(w, r, true)
}

// importRun is the shared body of preview (write=false) and commit
// (write=true). Both classify rows identically; commit additionally inserts /
// updates, merges custom properties, emits per-entity activities and a single
// batch audit event — all atomic.
func (h *Handler) importRun(w http.ResponseWriter, r *http.Request, write bool) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	p, _, r := principalFrom(r)
	spec, ok := specForParam(chi.URLParam(r, "entity"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown import entity")
		return
	}
	var body importMappingReq
	if !decodeImportBody(w, r, &body) {
		return
	}
	if body.Dedupe == "" {
		body.Dedupe = dedupeUpdate
	}
	switch body.Dedupe {
	case dedupeUpdate, dedupeSkip, dedupeCreate:
	default:
		writeErr(w, http.StatusBadRequest, "invalid dedupe policy")
		return
	}
	header, records, err := parseCSVText(body.CSV)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateMapping(spec, body.Mapping); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	eng := &importEngine{
		h:        h,
		spec:     spec,
		mapping:  body.Mapping,
		dedupe:   body.Dedupe,
		header:   header,
		records:  records,
		colIndex: indexColumns(header),
		seen:     map[string]bool{},
		write:    write,
		actor:    p.ActorType,
		wsID:     ws.ID,
	}

	if !write {
		// Dry run: read-only transaction, dedup lookups only.
		if e := readTx(r.Context(), h.Pool, ws.RoleName, eng.process); e != nil {
			h.Logger.ErrorContext(r.Context(), "import preview", "err", e)
			writeErr(w, http.StatusInternalServerError, "import preview failed")
			return
		}
		writeJSON(w, http.StatusOK, previewResp{Summary: eng.summary, Rows: eng.outcomes})
		return
	}

	// Commit: one write transaction. A fatal error rolls everything back.
	if e := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		if le := eng.process(tx); le != nil {
			return le
		}
		// Single batch audit event (ADR-007). Fail-closed: a failed insert
		// rolls back the whole import.
		return capability.EmitAudit(r.Context(), tx, "import.committed", ws.ID, p.ActorType, map[string]any{
			"entity":  spec.entity,
			"dedupe":  body.Dedupe,
			"total":   eng.summary.Total,
			"created": eng.summary.Create,
			"updated": eng.summary.Update,
			"skipped": eng.summary.Skip,
			"errored": eng.summary.Error,
		})
	}); e != nil {
		h.Logger.ErrorContext(r.Context(), "import commit", "err", e)
		writeErr(w, http.StatusInternalServerError, "import commit failed")
		return
	}

	writeJSON(w, http.StatusOK, commitResp{
		Summary:        eng.summary,
		ErrorReportCSV: eng.errorReportCSV(),
		AuditEvent:     "import.committed",
	})
}

// importEngine carries the per-request import state and the row processing
// loop shared by preview and commit.
type importEngine struct {
	h        *Handler
	spec     importEntitySpec
	mapping  map[string]string
	dedupe   string
	header   []string
	records  [][]string
	colIndex map[string]int
	seen     map[string]bool // in-batch dedup keys already encountered
	write    bool
	actor    string
	wsID     uuid.UUID

	stages   map[string]uuid.UUID // lowercased stage name -> id (deals only, lazy)
	summary  importSummary
	outcomes []rowOutcome
	errors   []rowOutcome // skipped + errored rows, for the downloadable report
}

// process runs the classification (and, when write, the mutations) over every
// data row inside tx.
func (e *importEngine) process(tx pgx.Tx) error {
	ctx := context.Background()
	q := sqlcgen.New(tx)

	if e.spec.entity == "deal" {
		if err := e.loadStages(ctx, tx); err != nil {
			return err
		}
	}
	var defs []importDef
	if e.spec.parentType != "" {
		var err error
		defs, err = loadImportDefs(ctx, tx, e.spec.parentType)
		if err != nil {
			return err
		}
	}
	defByKey := map[string]importDef{}
	for _, d := range defs {
		defByKey[d.Key] = d
	}

	for i, rec := range e.records {
		line := i + 2 // header is line 1
		e.summary.Total++

		core := e.coreValues(rec)
		custom, customErr := e.customValues(rec, defByKey)

		out := rowOutcome{Line: line, Label: e.rowLabel(core)}

		if customErr != "" {
			e.record(out, actionError, customErr)
			continue
		}
		if reason := e.validateCore(core); reason != "" {
			e.record(out, actionError, reason)
			continue
		}

		// Dedup match (contacts by email, companies by name/domain).
		var matchID uuid.UUID
		matched := false
		if e.spec.dedupe {
			id, found, err := e.lookupMatch(ctx, tx, core)
			if err != nil {
				return err
			}
			key := e.dedupeKey(core)
			if found {
				matchID, matched = id, true
			} else if key != "" && e.seen[key] {
				matched = true // earlier row in this batch already created it
			}
			if key != "" {
				e.seen[key] = true
			}
		}

		switch {
		case matched && e.dedupe == dedupeSkip:
			e.record(out, actionSkip, "matched existing record (skip policy)")
		case matched && e.dedupe == dedupeUpdate:
			if e.write {
				if err := e.applyUpdate(ctx, q, tx, matchID, core, custom); err != nil {
					e.record(out, actionError, err.Error())
					continue
				}
			}
			e.record(out, actionUpdate, "")
		default:
			// No match, or create policy: insert a new record.
			if e.write {
				if err := e.applyCreate(ctx, q, tx, core, custom); err != nil {
					e.record(out, actionError, err.Error())
					continue
				}
			}
			e.record(out, actionCreate, "")
		}
	}
	return nil
}

// record files a row outcome into the summary, the per-row list, and (for
// skip/error) the downloadable report.
func (e *importEngine) record(out rowOutcome, action, reason string) {
	out.Action = action
	out.Reason = reason
	switch action {
	case actionCreate:
		e.summary.Create++
	case actionUpdate:
		e.summary.Update++
	case actionSkip:
		e.summary.Skip++
		e.errors = append(e.errors, out)
	case actionError:
		e.summary.Error++
		e.errors = append(e.errors, out)
	}
	e.outcomes = append(e.outcomes, out)
}

// coreValues extracts the mapped core fields for one record, keyed by target
// field key, trimmed.
func (e *importEngine) coreValues(rec []string) map[string]string {
	out := map[string]string{}
	coreKeys := map[string]bool{}
	for _, f := range e.spec.coreFields {
		coreKeys[f.Key] = true
	}
	for col, target := range e.mapping {
		if !coreKeys[target] {
			continue
		}
		idx, ok := e.colIndex[col]
		if !ok || idx >= len(rec) {
			continue
		}
		out[target] = strings.TrimSpace(rec[idx])
	}
	return out
}

// customValues extracts mapped custom-property values, coercing each to its
// declared type. Returns a non-empty error string on the first invalid value.
func (e *importEngine) customValues(rec []string, defs map[string]importDef) (map[string]any, string) {
	out := map[string]any{}
	for col, target := range e.mapping {
		key, ok := strings.CutPrefix(target, customPropPrefix)
		if !ok {
			continue
		}
		def, known := defs[key]
		if !known {
			continue
		}
		idx, ok := e.colIndex[col]
		if !ok || idx >= len(rec) {
			continue
		}
		raw := strings.TrimSpace(rec[idx])
		if raw == "" {
			continue // empty cell → leave the property unset
		}
		v, err := coerceCustomValue(def, raw)
		if err != nil {
			return nil, err.Error()
		}
		out[key] = v
	}
	return out, ""
}

// rowLabel is a human hint shown next to a row outcome.
func (e *importEngine) rowLabel(core map[string]string) string {
	switch e.spec.entity {
	case "contact":
		name := strings.TrimSpace(core["first_name"] + " " + core["last_name"])
		if name != "" {
			return name
		}
		return core["email"]
	case "company":
		return core["name"]
	case "deal":
		return core["title"]
	}
	return ""
}

// validateCore enforces required fields and per-field formats. Returns a
// non-empty reason when the row is invalid.
func (e *importEngine) validateCore(core map[string]string) string {
	for _, f := range e.spec.coreFields {
		if f.Required && core[f.Key] == "" {
			return fmt.Sprintf("%s is required", f.Key)
		}
	}
	switch e.spec.entity {
	case "contact":
		if email := core["email"]; email != "" {
			if err := validateEmail(email); err != nil {
				return err.Error()
			}
		}
	case "company":
		if size := core["size"]; size != "" && !validCompanySize(size) {
			return fmt.Sprintf("invalid size %q", size)
		}
	case "deal":
		if amt := core["amount"]; amt != "" {
			if _, err := parseAmount(amt); err != nil {
				return err.Error()
			}
		}
		if cur := core["currency"]; cur != "" && len(cur) != 3 {
			return "currency must be a 3-letter ISO code"
		}
		if d := core["expected_close_date"]; d != "" && !validDate(d) {
			return "expected_close_date must be YYYY-MM-DD"
		}
		if st := core["stage"]; st != "" {
			if _, ok := e.stages[strings.ToLower(st)]; !ok {
				return fmt.Sprintf("unknown stage %q", st)
			}
		}
	}
	return ""
}

// dedupeKey is the in-batch dedup key for a row ("" when not dedup-eligible).
func (e *importEngine) dedupeKey(core map[string]string) string {
	switch e.spec.entity {
	case "contact":
		if core["email"] != "" {
			return "email:" + strings.ToLower(core["email"])
		}
	case "company":
		if core["domain"] != "" {
			return "domain:" + strings.ToLower(core["domain"])
		}
		if core["name"] != "" {
			return "name:" + strings.ToLower(core["name"])
		}
	}
	return ""
}

// lookupMatch finds an existing record to dedupe against, inside tx (so a
// commit sees rows inserted earlier in the same batch). Contacts match by
// email; companies by domain, then name.
func (e *importEngine) lookupMatch(ctx context.Context, tx pgx.Tx, core map[string]string) (uuid.UUID, bool, error) {
	var id uuid.UUID
	switch e.spec.entity {
	case "contact":
		email := core["email"]
		if email == "" {
			return uuid.Nil, false, nil
		}
		err := tx.QueryRow(ctx,
			`SELECT id FROM contacts WHERE email IS NOT NULL AND lower(email) = lower($1) ORDER BY created_at ASC LIMIT 1`,
			email).Scan(&id)
		if err == pgx.ErrNoRows {
			return uuid.Nil, false, nil
		}
		if err != nil {
			return uuid.Nil, false, err
		}
		return id, true, nil
	case "company":
		err := tx.QueryRow(ctx,
			`SELECT id FROM companies
			  WHERE (domain IS NOT NULL AND domain <> '' AND lower(domain) = lower($1))
			     OR lower(name) = lower($2)
			  ORDER BY created_at ASC LIMIT 1`,
			core["domain"], core["name"]).Scan(&id)
		if err == pgx.ErrNoRows {
			return uuid.Nil, false, nil
		}
		if err != nil {
			return uuid.Nil, false, err
		}
		return id, true, nil
	}
	return uuid.Nil, false, nil
}

// applyCreate inserts a new record (+ custom props + activity) inside tx.
func (e *importEngine) applyCreate(ctx context.Context, q *sqlcgen.Queries, tx pgx.Tx, core map[string]string, custom map[string]any) error {
	switch e.spec.entity {
	case "contact":
		row, err := q.CreateContact(ctx, sqlcgen.CreateContactParams{
			FirstName: core["first_name"],
			LastName:  core["last_name"],
			Email:     toText(strPtrOrNil(core["email"])),
			Phone:     toText(strPtrOrNil(core["phone"])),
		})
		if err != nil {
			return err
		}
		return e.afterWrite(ctx, tx, row.ID, custom, "entity.created")
	case "company":
		row, err := q.CreateCompany(ctx, sqlcgen.CreateCompanyParams{
			Name:     core["name"],
			Domain:   toText(strPtrOrNil(core["domain"])),
			Industry: toText(strPtrOrNil(core["industry"])),
			Size:     toText(strPtrOrNil(core["size"])),
		})
		if err != nil {
			return err
		}
		return e.afterWrite(ctx, tx, row.ID, custom, "entity.created")
	case "deal":
		row, err := q.CreateDeal(ctx, sqlcgen.CreateDealParams{
			Title:             core["title"],
			Amount:            toNumeric(amountPtr(core["amount"])),
			Currency:          toText(strPtrOrNil(core["currency"])),
			StageID:           e.stageID(core["stage"]),
			ExpectedCloseDate: toDate(strPtrOrNil(core["expected_close_date"])),
		})
		if err != nil {
			return err
		}
		return e.afterWrite(ctx, tx, row.ID, custom, "entity.created")
	}
	return nil
}

// applyUpdate overlays the mapped fields onto an existing record, preserving
// columns the import did not map, then merges custom props + emits activity.
func (e *importEngine) applyUpdate(ctx context.Context, q *sqlcgen.Queries, tx pgx.Tx, id uuid.UUID, core map[string]string, custom map[string]any) error {
	switch e.spec.entity {
	case "contact":
		existing, err := q.GetContact(ctx, id)
		if err != nil {
			return err
		}
		row, err := q.UpdateContact(ctx, sqlcgen.UpdateContactParams{
			ID:        id,
			FirstName: pickStr(core, "first_name", existing.FirstName),
			LastName:  pickStr(core, "last_name", existing.LastName),
			Email:     pickTextVal(core, "email", existing.Email),
			Phone:     pickTextVal(core, "phone", existing.Phone),
			CompanyID: existing.CompanyID,
			OwnerID:   existing.OwnerID,
		})
		if err != nil {
			return err
		}
		return e.afterWrite(ctx, tx, row.ID, custom, "entity.updated")
	case "company":
		existing, err := q.GetCompany(ctx, id)
		if err != nil {
			return err
		}
		row, err := q.UpdateCompany(ctx, sqlcgen.UpdateCompanyParams{
			ID:       id,
			Name:     pickStr(core, "name", existing.Name),
			Domain:   pickTextVal(core, "domain", existing.Domain),
			Industry: pickTextVal(core, "industry", existing.Industry),
			Size:     pickTextVal(core, "size", existing.Size),
			OwnerID:  existing.OwnerID,
		})
		if err != nil {
			return err
		}
		return e.afterWrite(ctx, tx, row.ID, custom, "entity.updated")
	}
	return nil
}

// afterWrite merges any custom properties and emits a per-entity activity row,
// both inside the import transaction.
func (e *importEngine) afterWrite(ctx context.Context, tx pgx.Tx, id uuid.UUID, custom map[string]any, event string) error {
	if e.spec.parentType != "" && len(custom) > 0 {
		if err := capability.MergeCustomProps(ctx, tx, e.spec.parentType, id, custom); err != nil {
			return err
		}
	}
	// The per-workspace activities table's actor_type CHECK predates the
	// integrator role; map it to human_api (the canonical integrator trail is
	// the batch row in core.audit_log). Mirrors capability.emitEntityActivity.
	actor := e.actor
	if actor == "" || actor == capability.ActorTypeIntegrator {
		actor = capability.ActorTypeHumanAPI
	}
	return capability.EmitActivity(ctx, tx, e.spec.entity, id, event, actor, "import", map[string]any{
		"source": "csv_import",
	})
}

// stageID resolves a stage name (case-insensitive) to its id, or a null UUID.
func (e *importEngine) stageID(name string) uuid.NullUUID {
	if name == "" {
		return uuid.NullUUID{}
	}
	if id, ok := e.stages[strings.ToLower(name)]; ok {
		return uuid.NullUUID{UUID: id, Valid: true}
	}
	return uuid.NullUUID{}
}

// loadStages caches the workspace's pipeline stages by lowercased name.
func (e *importEngine) loadStages(ctx context.Context, tx pgx.Tx) error {
	e.stages = map[string]uuid.UUID{}
	rows, err := tx.Query(ctx, `SELECT id, name FROM pipeline_stages`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		e.stages[strings.ToLower(name)] = id
	}
	return rows.Err()
}

// errorReportCSV renders the skipped/errored rows as a downloadable CSV.
func (e *importEngine) errorReportCSV() string {
	var sb strings.Builder
	cw := csv.NewWriter(&sb)
	_ = cw.Write([]string{"line", "action", "label", "reason"})
	for _, o := range e.errors {
		_ = cw.Write([]string{strconv.Itoa(o.Line), o.Action, o.Label, o.Reason})
	}
	cw.Flush()
	return sb.String()
}

// =========================================================================
// CSV + mapping helpers
// =========================================================================

// parseCSVText parses CSV text into a header row and the data rows. A
// variable field count is tolerated (ragged rows are common in exports).
func parseCSVText(text string) ([]string, [][]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil, fmt.Errorf("empty CSV")
	}
	rdr := csv.NewReader(strings.NewReader(text))
	rdr.FieldsPerRecord = -1
	rdr.TrimLeadingSpace = true
	records, err := rdr.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("could not parse CSV: %v", err)
	}
	if len(records) == 0 {
		return nil, nil, fmt.Errorf("CSV has no rows")
	}
	header := records[0]
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}
	return header, records[1:], nil
}

// indexColumns maps each header name to its column index (first wins on dups).
func indexColumns(header []string) map[string]int {
	idx := map[string]int{}
	for i, h := range header {
		if _, exists := idx[h]; !exists {
			idx[h] = i
		}
	}
	return idx
}

// validateMapping rejects targets that name neither a core field nor a known
// "cf_" custom-property column. Empty/absent targets are allowed (ignored).
func validateMapping(spec importEntitySpec, mapping map[string]string) error {
	core := map[string]bool{}
	for _, f := range spec.coreFields {
		core[f.Key] = true
	}
	for col, target := range mapping {
		if target == "" {
			continue
		}
		if strings.HasPrefix(target, customPropPrefix) {
			if spec.parentType == "" {
				return fmt.Errorf("column %q maps to a custom property but %s has none", col, spec.plural)
			}
			continue
		}
		if !core[target] {
			return fmt.Errorf("column %q maps to unknown field %q", col, target)
		}
	}
	return nil
}

// suggestMapping makes a best-effort column → target guess by normalising the
// header against core field keys/labels and the custom-property keys.
func suggestMapping(header []string, spec importEntitySpec, defs []importDef) map[string]string {
	out := map[string]string{}
	for _, col := range header {
		norm := normalizeKey(col)
		if norm == "" {
			continue
		}
		// Core field by key or label.
		matched := false
		for _, f := range spec.coreFields {
			if norm == f.Key || norm == normalizeKey(f.Label) || coreAlias(f.Key, norm) {
				out[col] = f.Key
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		// Custom property by key or label.
		for _, d := range defs {
			if norm == normalizeKey(d.Key) || norm == normalizeKey(d.Label) {
				out[col] = customPropPrefix + d.Key
				break
			}
		}
	}
	return out
}

// coreAlias matches a few common header spellings to canonical field keys.
func coreAlias(key, norm string) bool {
	aliases := map[string][]string{
		"first_name": {"firstname", "prenom", "given_name"},
		"last_name":  {"lastname", "nom", "surname", "family_name"},
		"email":      {"e_mail", "mail", "courriel"},
		"phone":      {"telephone", "tel", "mobile", "phone_number"},
		"name":       {"company", "company_name", "entreprise", "raison_sociale"},
		"domain":     {"website", "site", "url", "site_web"},
		"industry":   {"secteur", "sector"},
		"title":      {"deal", "deal_name", "opportunity", "intitule"},
		"amount":     {"montant", "value", "valeur"},
		"currency":   {"devise"},
		"stage":      {"etape", "phase", "pipeline_stage"},
	}
	for _, a := range aliases[key] {
		if norm == a {
			return true
		}
	}
	return false
}

// normalizeKey lowercases, strips accents-light, and collapses separators to
// underscores so "First Name", "first-name" and "first_name" all match.
func normalizeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

// =========================================================================
// value coercion / validation
// =========================================================================

// coerceCustomValue converts a raw CSV string to the typed value the
// definition declares, validating enums/dates/numbers explicitly (never the
// Python-style bool(string) trap — booleans match an explicit token set).
func coerceCustomValue(def importDef, raw string) (any, error) {
	switch def.PropertyType {
	case "number":
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a number", def.Key, raw)
		}
		return f, nil
	case "boolean":
		switch strings.ToLower(raw) {
		case "true", "1", "yes", "oui", "y", "vrai":
			return true, nil
		case "false", "0", "no", "non", "n", "faux":
			return false, nil
		default:
			return nil, fmt.Errorf("%s: %q is not a boolean", def.Key, raw)
		}
	case "date":
		if !validDate(raw) {
			return nil, fmt.Errorf("%s: %q is not a date (YYYY-MM-DD)", def.Key, raw)
		}
		return raw, nil
	case "enum":
		for _, a := range def.Allowed {
			if a == raw {
				return raw, nil
			}
		}
		return nil, fmt.Errorf("%s: %q is not an allowed value", def.Key, raw)
	case "json":
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, fmt.Errorf("%s: invalid JSON", def.Key)
		}
		return v, nil
	default: // string
		return raw, nil
	}
}

// loadImportDefs reads the workspace's custom-property definitions inside tx
// (search_path pinned). Mirrors metadata.ListDefinitions but tx-scoped so the
// import shares one transaction.
func loadImportDefs(ctx context.Context, tx pgx.Tx, parentType string) ([]importDef, error) {
	rows, err := tx.Query(ctx,
		`SELECT property_key, property_type, allowed_values
		   FROM custom_property_definitions WHERE parent_type = $1 ORDER BY property_key`,
		parentType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []importDef
	for rows.Next() {
		var d importDef
		var allowedRaw []byte
		if err := rows.Scan(&d.Key, &d.PropertyType, &allowedRaw); err != nil {
			return nil, err
		}
		if len(allowedRaw) > 0 {
			_ = json.Unmarshal(allowedRaw, &d.Allowed)
		}
		d.Label = prettifyKey(d.Key)
		out = append(out, d)
	}
	if out == nil {
		out = []importDef{}
	}
	return out, rows.Err()
}

// prettifyKey turns "lead_source" into "Lead source" for display.
func prettifyKey(key string) string {
	s := strings.ReplaceAll(key, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// =========================================================================
// small value helpers
// =========================================================================

func decodeImportBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBodySize)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func amountPtr(s string) *float64 {
	if s == "" {
		return nil
	}
	f, err := parseAmount(s)
	if err != nil {
		return nil
	}
	return &f
}

// parseAmount parses a monetary string, tolerating a comma decimal separator
// and surrounding spaces ("1 234,50" → 1234.50).
func parseAmount(s string) (float64, error) {
	clean := strings.ReplaceAll(s, " ", "")
	clean = strings.ReplaceAll(clean, " ", "")
	// If there's a comma and no dot, treat comma as the decimal separator.
	if strings.Contains(clean, ",") && !strings.Contains(clean, ".") {
		clean = strings.ReplaceAll(clean, ",", ".")
	} else {
		clean = strings.ReplaceAll(clean, ",", "")
	}
	f, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q is not a number", s)
	}
	return f, nil
}

// pickStr returns the mapped value for key when present and non-empty,
// otherwise the existing value (for COALESCE-style preserve-on-update).
func pickStr(core map[string]string, key, existing string) string {
	if v, ok := core[key]; ok && v != "" {
		return v
	}
	return existing
}

// pickText returns a pgtype.Text from the mapped value when the key was
// mapped and non-empty, otherwise preserves the existing pgtype.Text (so an
// unmapped column is never nulled by an update).
func pickTextVal(core map[string]string, key string, existing pgtype.Text) pgtype.Text {
	if v, ok := core[key]; ok && v != "" {
		return pgtype.Text{String: v, Valid: true}
	}
	return existing
}

// validateEmail returns a non-nil error when the address is syntactically
// invalid. Empty input is always accepted (callers check blank separately).
func validateEmail(email string) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("invalid email %q", email)
	}
	return nil
}

// validCompanySize returns true when size is one of the canonical buckets
// (mirrors domain.validCompanySizes).
func validCompanySize(size string) bool {
	switch size {
	case "1-10", "11-50", "51-200", "201-1000", "1000+":
		return true
	}
	return false
}

// validDate returns true when s is a YYYY-MM-DD date string.
func validDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
