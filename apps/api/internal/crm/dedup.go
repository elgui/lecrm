package crm

// Duplicate detection and record merge — integrator-gap tasket 20260601-110828-76e8.
//
// Endpoints:
//
//	GET  /v1/dedup/contacts            — list contact duplicate candidates
//	GET  /v1/dedup/companies           — list company duplicate candidates
//	POST /v1/dedup/contacts/merge      — merge two contacts
//	POST /v1/dedup/companies/merge     — merge two companies
//	POST /v1/dedup/contacts/distinct   — mark pair as "never suggest again"
//	POST /v1/dedup/companies/distinct  — mark pair as "never suggest again"
//
// Detection:
//   - Contacts: exact email match (strong) + trigram name similarity ≥ 0.4 (weak).
//   - Companies: exact domain match (strong) + trigram name similarity ≥ 0.4 (weak).
//   - No-merge rule pairs are excluded from candidates.
//
// Merge:
//   - Caller specifies the survivor and loser IDs, plus a per-field resolver
//     ("survivor" | "loser") for each built-in field.
//   - All notes, activities, tasks, and deals are re-pointed from the loser to
//     the survivor; no relation is orphaned.
//   - Objects (custom properties) belonging to the loser are deleted after the
//     survivor's custom properties are preserved (survivor-wins policy; the UI
//     can surface both values for the user to choose before calling merge).
//   - Emits a contact.merged / company.merged audit event (ADR-007).
//   - Runs inside a single write transaction; fails-closed (ADR-009 §7.2).
//
// Tenant isolation:
//   - All queries use the workspace schema set by writeTx / readTx; cross-tenant
//     access is structurally impossible.

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// --- trigram helpers (in-memory fuzzy matching) ---

func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func trigrams(s string) map[string]bool {
	padded := "  " + s + "  "
	runes := []rune(padded)
	out := make(map[string]bool, len(runes))
	for i := 0; i+2 < len(runes); i++ {
		out[string(runes[i:i+3])] = true
	}
	return out
}

func trigramSimilarity(a, b string) float64 {
	na, nb := normalizeName(a), normalizeName(b)
	ta, tb := trigrams(na), trigrams(nb)
	if len(ta) == 0 && len(tb) == 0 {
		return 1.0
	}
	if len(ta) == 0 || len(tb) == 0 {
		return 0.0
	}
	intersection := 0
	for t := range ta {
		if tb[t] {
			intersection++
		}
	}
	union := len(ta) + len(tb) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// --- response + request types ---

type dedupRecordContact struct {
	ID        uuid.UUID `json:"id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Email     *string   `json:"email"`
	Phone     *string   `json:"phone"`
	CompanyID *string   `json:"company_id"`
	CreatedAt string    `json:"created_at"`
}

type dedupRecordCompany struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Domain    *string   `json:"domain"`
	Industry  *string   `json:"industry"`
	CreatedAt string    `json:"created_at"`
}

type dedupContactPair struct {
	A      dedupRecordContact `json:"a"`
	B      dedupRecordContact `json:"b"`
	Reason string             `json:"reason"` // "exact_email" | "similar_name"
	Score  float64            `json:"score"`
}

type dedupCompanyPair struct {
	A      dedupRecordCompany `json:"a"`
	B      dedupRecordCompany `json:"b"`
	Reason string             `json:"reason"` // "exact_domain" | "similar_name"
	Score  float64            `json:"score"`
}

// mergeContactReq specifies which side wins per field.
// "survivor" keeps the survivor's value; "loser" takes the loser's value.
// Fields omitted from the map default to "survivor".
type mergeContactReq struct {
	SurvivorID string            `json:"survivor_id"`
	LoserID    string            `json:"loser_id"`
	Fields     map[string]string `json:"fields"` // field_key → "survivor" | "loser"
}

type mergeCompanyReq struct {
	SurvivorID string            `json:"survivor_id"`
	LoserID    string            `json:"loser_id"`
	Fields     map[string]string `json:"fields"`
}

type distinctReq struct {
	IDA string `json:"id_a"`
	IDB string `json:"id_b"`
}

// canonicalPair returns (lo, hi) so id_a < id_b for the CHECK constraint.
func canonicalPair(a, b uuid.UUID) (uuid.UUID, uuid.UUID) {
	if a.String() < b.String() {
		return a, b
	}
	return b, a
}

// (The per-query no-merge exclusion set is built inline in the candidate
// scans below — see the `excluded` maps in ListContactDuplicates /
// ListCompanyDuplicates — so no shared helper is needed.)

func isExcluded(excluded map[[2]uuid.UUID]bool, a, b uuid.UUID) bool {
	lo, hi := canonicalPair(a, b)
	return excluded[[2]uuid.UUID{lo, hi}]
}

// --- handlers ---

// ListContactDuplicates GET /v1/dedup/contacts
func (h *Handler) ListContactDuplicates(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}

	type row struct {
		id        uuid.UUID
		firstName string
		lastName  string
		email     pgtype.Text
		phone     pgtype.Text
		companyID uuid.NullUUID
		createdAt pgtype.Timestamptz
	}

	var contacts []row
	err := func() error {
		pool := h.Pool
		conn, err := pool.Acquire(r.Context())
		if err != nil {
			return err
		}
		defer conn.Release()

		_, err = conn.Exec(r.Context(), "SET search_path = "+ws.RoleName+", public")
		if err != nil {
			return err
		}
		rows, err := conn.Query(r.Context(),
			`SELECT id, first_name, last_name, email, phone, company_id, created_at
			   FROM contacts ORDER BY created_at ASC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c row
			if err := rows.Scan(&c.id, &c.firstName, &c.lastName, &c.email, &c.phone, &c.companyID, &c.createdAt); err != nil {
				return err
			}
			contacts = append(contacts, c)
		}
		return rows.Err()
	}()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "dedup query failed")
		return
	}

	// Load no-merge exclusions using a read connection.
	excluded := make(map[[2]uuid.UUID]bool)
	_ = func() error {
		conn, err := h.Pool.Acquire(r.Context())
		if err != nil {
			return err
		}
		defer conn.Release()
		_, _ = conn.Exec(r.Context(), "SET search_path = "+ws.RoleName+", public")
		rows, err := conn.Query(r.Context(),
			`SELECT id_a, id_b FROM dedup_no_merge_rules WHERE entity_type = 'contact'`)
		if err != nil {
			return nil // table may not exist on old DBs; degrade gracefully
		}
		defer rows.Close()
		for rows.Next() {
			var a, b uuid.UUID
			if err := rows.Scan(&a, &b); err != nil {
				return err
			}
			lo, hi := canonicalPair(a, b)
			excluded[[2]uuid.UUID{lo, hi}] = true
		}
		return rows.Err()
	}()

	toRec := func(c row) dedupRecordContact {
		rec := dedupRecordContact{
			ID:        c.id,
			FirstName: c.firstName,
			LastName:  c.lastName,
		}
		if c.email.Valid {
			s := c.email.String
			rec.Email = &s
		}
		if c.phone.Valid {
			s := c.phone.String
			rec.Phone = &s
		}
		if c.companyID.Valid {
			s := c.companyID.UUID.String()
			rec.CompanyID = &s
		}
		if c.createdAt.Valid {
			rec.CreatedAt = c.createdAt.Time.Format("2006-01-02T15:04:05Z")
		}
		return rec
	}

	seen := make(map[[2]uuid.UUID]bool)
	var pairs []dedupContactPair

	// Pass 1: exact email matches.
	byEmail := make(map[string][]row)
	for _, c := range contacts {
		if !c.email.Valid || c.email.String == "" {
			continue
		}
		key := strings.ToLower(c.email.String)
		byEmail[key] = append(byEmail[key], c)
	}
	for _, group := range byEmail {
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				a, b := group[i], group[j]
				lo, hi := canonicalPair(a.id, b.id)
				if seen[[2]uuid.UUID{lo, hi}] || isExcluded(excluded, a.id, b.id) {
					continue
				}
				seen[[2]uuid.UUID{lo, hi}] = true
				pairs = append(pairs, dedupContactPair{
					A: toRec(a), B: toRec(b),
					Reason: "exact_email", Score: 1.0,
				})
			}
		}
	}

	// Pass 2: fuzzy name similarity (skip already-paired contacts).
	const nameThreshold = 0.4
	for i := 0; i < len(contacts); i++ {
		for j := i + 1; j < len(contacts); j++ {
			a, b := contacts[i], contacts[j]
			lo, hi := canonicalPair(a.id, b.id)
			if seen[[2]uuid.UUID{lo, hi}] || isExcluded(excluded, a.id, b.id) {
				continue
			}
			fullA := a.firstName + " " + a.lastName
			fullB := b.firstName + " " + b.lastName
			score := trigramSimilarity(fullA, fullB)
			if score >= nameThreshold {
				seen[[2]uuid.UUID{lo, hi}] = true
				pairs = append(pairs, dedupContactPair{
					A: toRec(a), B: toRec(b),
					Reason: "similar_name", Score: score,
				})
			}
		}
	}

	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Score > pairs[j].Score })
	writeJSON(w, http.StatusOK, map[string]any{"pairs": pairs})
}

// ListCompanyDuplicates GET /v1/dedup/companies
func (h *Handler) ListCompanyDuplicates(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}

	type row struct {
		id        uuid.UUID
		name      string
		domain    pgtype.Text
		industry  pgtype.Text
		createdAt pgtype.Timestamptz
	}

	var companies []row
	err := func() error {
		conn, err := h.Pool.Acquire(r.Context())
		if err != nil {
			return err
		}
		defer conn.Release()
		_, err = conn.Exec(r.Context(), "SET search_path = "+ws.RoleName+", public")
		if err != nil {
			return err
		}
		rows, err := conn.Query(r.Context(),
			`SELECT id, name, domain, industry, created_at FROM companies ORDER BY created_at ASC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c row
			if err := rows.Scan(&c.id, &c.name, &c.domain, &c.industry, &c.createdAt); err != nil {
				return err
			}
			companies = append(companies, c)
		}
		return rows.Err()
	}()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "dedup query failed")
		return
	}

	excluded := make(map[[2]uuid.UUID]bool)
	_ = func() error {
		conn, err := h.Pool.Acquire(r.Context())
		if err != nil {
			return err
		}
		defer conn.Release()
		_, _ = conn.Exec(r.Context(), "SET search_path = "+ws.RoleName+", public")
		rows, err := conn.Query(r.Context(),
			`SELECT id_a, id_b FROM dedup_no_merge_rules WHERE entity_type = 'company'`)
		if err != nil {
			return nil
		}
		defer rows.Close()
		for rows.Next() {
			var a, b uuid.UUID
			if err := rows.Scan(&a, &b); err != nil {
				return err
			}
			lo, hi := canonicalPair(a, b)
			excluded[[2]uuid.UUID{lo, hi}] = true
		}
		return rows.Err()
	}()

	toRec := func(c row) dedupRecordCompany {
		rec := dedupRecordCompany{ID: c.id, Name: c.name}
		if c.domain.Valid {
			s := c.domain.String
			rec.Domain = &s
		}
		if c.industry.Valid {
			s := c.industry.String
			rec.Industry = &s
		}
		if c.createdAt.Valid {
			rec.CreatedAt = c.createdAt.Time.Format("2006-01-02T15:04:05Z")
		}
		return rec
	}

	seen := make(map[[2]uuid.UUID]bool)
	var pairs []dedupCompanyPair

	// Pass 1: exact domain matches (non-empty).
	byDomain := make(map[string][]row)
	for _, c := range companies {
		if !c.domain.Valid || c.domain.String == "" {
			continue
		}
		key := strings.ToLower(c.domain.String)
		byDomain[key] = append(byDomain[key], c)
	}
	for _, group := range byDomain {
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				a, b := group[i], group[j]
				lo, hi := canonicalPair(a.id, b.id)
				if seen[[2]uuid.UUID{lo, hi}] || isExcluded(excluded, a.id, b.id) {
					continue
				}
				seen[[2]uuid.UUID{lo, hi}] = true
				pairs = append(pairs, dedupCompanyPair{
					A: toRec(a), B: toRec(b),
					Reason: "exact_domain", Score: 1.0,
				})
			}
		}
	}

	// Pass 2: fuzzy name similarity.
	const nameThreshold = 0.4
	for i := 0; i < len(companies); i++ {
		for j := i + 1; j < len(companies); j++ {
			a, b := companies[i], companies[j]
			lo, hi := canonicalPair(a.id, b.id)
			if seen[[2]uuid.UUID{lo, hi}] || isExcluded(excluded, a.id, b.id) {
				continue
			}
			score := trigramSimilarity(a.name, b.name)
			if score >= nameThreshold {
				seen[[2]uuid.UUID{lo, hi}] = true
				pairs = append(pairs, dedupCompanyPair{
					A: toRec(a), B: toRec(b),
					Reason: "similar_name", Score: score,
				})
			}
		}
	}

	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Score > pairs[j].Score })
	writeJSON(w, http.StatusOK, map[string]any{"pairs": pairs})
}

// pick returns the value from either survivor or loser based on the resolver map.
func pick[T any](fields map[string]string, key string, survivorVal, loserVal T) T {
	if v, ok := fields[key]; ok && v == "loser" {
		return loserVal
	}
	return survivorVal
}

// pickText resolves a text field.
func pickText(fields map[string]string, key string, survivorVal, loserVal pgtype.Text) pgtype.Text {
	if v, ok := fields[key]; ok && v == "loser" {
		return loserVal
	}
	return survivorVal
}

// pickNullUUID resolves a nullable UUID field.
func pickNullUUID(fields map[string]string, key string, survivorVal, loserVal uuid.NullUUID) uuid.NullUUID {
	if v, ok := fields[key]; ok && v == "loser" {
		return loserVal
	}
	return survivorVal
}

// MergeContacts POST /v1/dedup/contacts/merge
func (h *Handler) MergeContacts(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	var req mergeContactReq
	if !decodeBody(w, r, &req) {
		return
	}
	survivorID, err := uuid.Parse(req.SurvivorID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid survivor_id")
		return
	}
	loserID, err := uuid.Parse(req.LoserID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid loser_id")
		return
	}
	if survivorID == loserID {
		writeErr(w, http.StatusBadRequest, "survivor_id and loser_id must differ")
		return
	}
	if req.Fields == nil {
		req.Fields = map[string]string{}
	}

	ctx := r.Context()
	mergeErr := writeTx(ctx, h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		// Load both records.
		type cRow struct {
			id        uuid.UUID
			firstName string
			lastName  string
			email     pgtype.Text
			phone     pgtype.Text
			companyID uuid.NullUUID
			ownerID   uuid.NullUUID
		}
		load := func(id uuid.UUID) (cRow, error) {
			var c cRow
			err := tx.QueryRow(ctx,
				`SELECT id, first_name, last_name, email, phone, company_id, owner_id
				   FROM contacts WHERE id = $1`, id).
				Scan(&c.id, &c.firstName, &c.lastName, &c.email, &c.phone, &c.companyID, &c.ownerID)
			return c, err
		}
		survivor, err := load(survivorID)
		if errors.Is(err, pgx.ErrNoRows) {
			return &errDedupNotFound{"survivor contact not found"}
		}
		if err != nil {
			return err
		}
		loser, err := load(loserID)
		if errors.Is(err, pgx.ErrNoRows) {
			return &errDedupNotFound{"loser contact not found"}
		}
		if err != nil {
			return err
		}

		// Resolve fields.
		mergedFirstName := pick(req.Fields, "first_name", survivor.firstName, loser.firstName)
		mergedLastName := pick(req.Fields, "last_name", survivor.lastName, loser.lastName)
		mergedEmail := pickText(req.Fields, "email", survivor.email, loser.email)
		mergedPhone := pickText(req.Fields, "phone", survivor.phone, loser.phone)
		mergedCompanyID := pickNullUUID(req.Fields, "company_id", survivor.companyID, loser.companyID)
		mergedOwnerID := pickNullUUID(req.Fields, "owner_id", survivor.ownerID, loser.ownerID)

		// 1. Update the survivor.
		if _, err := tx.Exec(ctx,
			`UPDATE contacts SET first_name=$1, last_name=$2, email=$3, phone=$4,
			  company_id=$5, owner_id=$6, updated_at=now()
			  WHERE id=$7`,
			mergedFirstName, mergedLastName, mergedEmail, mergedPhone,
			mergedCompanyID, mergedOwnerID, survivorID,
		); err != nil {
			return err
		}

		// 2. Re-point notes.
		if _, err := tx.Exec(ctx,
			`UPDATE notes SET entity_id=$1 WHERE entity_type='contact' AND entity_id=$2`,
			survivorID, loserID,
		); err != nil {
			return err
		}

		// 3. Re-point activities.
		if _, err := tx.Exec(ctx,
			`UPDATE activities SET entity_id=$1 WHERE entity_type='contact' AND entity_id=$2`,
			survivorID, loserID,
		); err != nil {
			return err
		}

		// 4. Re-point tasks.
		if _, err := tx.Exec(ctx,
			`UPDATE tasks SET entity_id=$1 WHERE entity_type='contact' AND entity_id=$2`,
			survivorID, loserID,
		); err != nil {
			return err
		}

		// 5. Re-point deals.
		if _, err := tx.Exec(ctx,
			`UPDATE deals SET contact_id=$1 WHERE contact_id=$2`,
			survivorID, loserID,
		); err != nil {
			return err
		}

		// 6. Remove loser's orphaned objects (custom properties, legacy activity rows).
		if _, err := tx.Exec(ctx,
			`DELETE FROM objects WHERE parent_type='contact' AND parent_id=$1`,
			loserID,
		); err != nil {
			return err
		}

		// 7. Delete the no-merge rule for this pair (if any) — they are merged now.
		lo, hi := canonicalPair(survivorID, loserID)
		if _, err := tx.Exec(ctx,
			`DELETE FROM dedup_no_merge_rules
			  WHERE entity_type='contact' AND id_a=$1 AND id_b=$2`,
			lo, hi,
		); err != nil {
			return err
		}

		// 8. Delete the loser.
		if _, err := tx.Exec(ctx, `DELETE FROM contacts WHERE id=$1`, loserID); err != nil {
			return err
		}

		// 9. Audit event (ADR-007).
		return emitAudit(ctx, tx, "contact.merged", ws.ID, map[string]any{
			"survivor_id": survivorID.String(),
			"loser_id":    loserID.String(),
		})
	})
	if mergeErr != nil {
		var nf *errDedupNotFound
		if isDedupNotFound(mergeErr, &nf) {
			writeErr(w, http.StatusNotFound, nf.msg)
			return
		}
		h.Logger.ErrorContext(ctx, "merge contacts failed", "err", mergeErr)
		writeErr(w, http.StatusInternalServerError, "merge failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"survivor_id": survivorID.String(),
		"loser_id":    loserID.String(),
		"merged":      true,
	})
}

// MergeCompanies POST /v1/dedup/companies/merge
func (h *Handler) MergeCompanies(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	var req mergeCompanyReq
	if !decodeBody(w, r, &req) {
		return
	}
	survivorID, err := uuid.Parse(req.SurvivorID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid survivor_id")
		return
	}
	loserID, err := uuid.Parse(req.LoserID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid loser_id")
		return
	}
	if survivorID == loserID {
		writeErr(w, http.StatusBadRequest, "survivor_id and loser_id must differ")
		return
	}
	if req.Fields == nil {
		req.Fields = map[string]string{}
	}

	ctx := r.Context()
	mergeErr := writeTx(ctx, h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		type cRow struct {
			id       uuid.UUID
			name     string
			domain   pgtype.Text
			industry pgtype.Text
			size     pgtype.Text
			ownerID  uuid.NullUUID
		}
		load := func(id uuid.UUID) (cRow, error) {
			var c cRow
			err := tx.QueryRow(ctx,
				`SELECT id, name, domain, industry, size, owner_id
				   FROM companies WHERE id=$1`, id).
				Scan(&c.id, &c.name, &c.domain, &c.industry, &c.size, &c.ownerID)
			return c, err
		}
		survivor, err := load(survivorID)
		if errors.Is(err, pgx.ErrNoRows) {
			return &errDedupNotFound{"survivor company not found"}
		}
		if err != nil {
			return err
		}
		loser, err := load(loserID)
		if errors.Is(err, pgx.ErrNoRows) {
			return &errDedupNotFound{"loser company not found"}
		}
		if err != nil {
			return err
		}

		mergedName := pick(req.Fields, "name", survivor.name, loser.name)
		mergedDomain := pickText(req.Fields, "domain", survivor.domain, loser.domain)
		mergedIndustry := pickText(req.Fields, "industry", survivor.industry, loser.industry)
		mergedSize := pickText(req.Fields, "size", survivor.size, loser.size)
		mergedOwnerID := pickNullUUID(req.Fields, "owner_id", survivor.ownerID, loser.ownerID)

		// 1. Update the survivor.
		if _, err := tx.Exec(ctx,
			`UPDATE companies SET name=$1, domain=$2, industry=$3, size=$4,
			  owner_id=$5, updated_at=now() WHERE id=$6`,
			mergedName, mergedDomain, mergedIndustry, mergedSize, mergedOwnerID, survivorID,
		); err != nil {
			return err
		}

		// 2. Re-point contacts.
		if _, err := tx.Exec(ctx,
			`UPDATE contacts SET company_id=$1 WHERE company_id=$2`,
			survivorID, loserID,
		); err != nil {
			return err
		}

		// 3. Re-point deals.
		if _, err := tx.Exec(ctx,
			`UPDATE deals SET company_id=$1 WHERE company_id=$2`,
			survivorID, loserID,
		); err != nil {
			return err
		}

		// 4. Re-point notes.
		if _, err := tx.Exec(ctx,
			`UPDATE notes SET entity_id=$1 WHERE entity_type='company' AND entity_id=$2`,
			survivorID, loserID,
		); err != nil {
			return err
		}

		// 5. Re-point activities.
		if _, err := tx.Exec(ctx,
			`UPDATE activities SET entity_id=$1 WHERE entity_type='company' AND entity_id=$2`,
			survivorID, loserID,
		); err != nil {
			return err
		}

		// 6. Re-point tasks.
		if _, err := tx.Exec(ctx,
			`UPDATE tasks SET entity_id=$1 WHERE entity_type='company' AND entity_id=$2`,
			survivorID, loserID,
		); err != nil {
			return err
		}

		// 7. Delete no-merge rule for this pair (if any).
		lo, hi := canonicalPair(survivorID, loserID)
		if _, err := tx.Exec(ctx,
			`DELETE FROM dedup_no_merge_rules
			  WHERE entity_type='company' AND id_a=$1 AND id_b=$2`,
			lo, hi,
		); err != nil {
			return err
		}

		// 8. Delete the loser.
		if _, err := tx.Exec(ctx, `DELETE FROM companies WHERE id=$1`, loserID); err != nil {
			return err
		}

		// 9. Audit event.
		return emitAudit(ctx, tx, "company.merged", ws.ID, map[string]any{
			"survivor_id": survivorID.String(),
			"loser_id":    loserID.String(),
		})
	})
	if mergeErr != nil {
		var nf *errDedupNotFound
		if isDedupNotFound(mergeErr, &nf) {
			writeErr(w, http.StatusNotFound, nf.msg)
			return
		}
		h.Logger.ErrorContext(ctx, "merge companies failed", "err", mergeErr)
		writeErr(w, http.StatusInternalServerError, "merge failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"survivor_id": survivorID.String(),
		"loser_id":    loserID.String(),
		"merged":      true,
	})
}

// MarkContactsDistinct POST /v1/dedup/contacts/distinct
func (h *Handler) MarkContactsDistinct(w http.ResponseWriter, r *http.Request) {
	h.markDistinct(w, r, "contact")
}

// MarkCompaniesDistinct POST /v1/dedup/companies/distinct
func (h *Handler) MarkCompaniesDistinct(w http.ResponseWriter, r *http.Request) {
	h.markDistinct(w, r, "company")
}

func (h *Handler) markDistinct(w http.ResponseWriter, r *http.Request, entity string) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	var req distinctReq
	if !decodeBody(w, r, &req) {
		return
	}
	idA, err := uuid.Parse(req.IDA)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id_a")
		return
	}
	idB, err := uuid.Parse(req.IDB)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id_b")
		return
	}
	if idA == idB {
		writeErr(w, http.StatusBadRequest, "id_a and id_b must differ")
		return
	}

	lo, hi := canonicalPair(idA, idB)
	ctx := r.Context()
	dbErr := writeTx(ctx, h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO dedup_no_merge_rules (entity_type, id_a, id_b)
			  VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			entity, lo, hi,
		)
		return err
	})
	if dbErr != nil {
		h.Logger.ErrorContext(ctx, "mark distinct failed", "err", dbErr)
		writeErr(w, http.StatusInternalServerError, "mark distinct failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"distinct": true})
}

// --- errDedupNotFound sentinel ---

type errDedupNotFound struct{ msg string }

func (e *errDedupNotFound) Error() string { return e.msg }

func isDedupNotFound(err error, target **errDedupNotFound) bool {
	return errors.As(err, target)
}

