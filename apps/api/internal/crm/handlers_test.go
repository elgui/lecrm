package crm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// =========================================================================
// encodeCursor / decodeCursor
// =========================================================================

func TestEncodeDecode_Cursor_RoundTrip(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	ts := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	encoded := encodeCursor(ts, id)
	if encoded == "" {
		t.Fatal("encodeCursor returned empty string")
	}

	gotTS, gotID, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decodeCursor round-trip error: %v", err)
	}
	if !gotTS.Valid {
		t.Fatal("decodeCursor: Timestamptz.Valid should be true")
	}
	if !gotTS.Time.Equal(ts) {
		t.Errorf("time: got %v, want %v", gotTS.Time, ts)
	}
	if gotID != id {
		t.Errorf("id: got %v, want %v", gotID, id)
	}
}

func TestEncodeDecode_Cursor_ZeroValues(t *testing.T) {
	// Zero time and nil UUID should still encode/decode cleanly.
	encoded := encodeCursor(time.Time{}, uuid.Nil)
	if encoded == "" {
		t.Fatal("encodeCursor with zero values returned empty string")
	}
	gotTS, gotID, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decodeCursor zero-value error: %v", err)
	}
	if !gotTS.Valid {
		t.Fatal("decodeCursor: Timestamptz.Valid should be true for zero time")
	}
	if gotID != uuid.Nil {
		t.Errorf("id: got %v, want uuid.Nil", gotID)
	}
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	_, _, err := decodeCursor("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestDecodeCursor_ValidBase64_InvalidJSON(t *testing.T) {
	// Valid base64 but not valid cursor JSON.
	garbage := base64.URLEncoding.EncodeToString([]byte("this is not json {{{"))
	_, _, err := decodeCursor(garbage)
	if err == nil {
		t.Fatal("expected error for invalid JSON inside base64, got nil")
	}
}

func TestDecodeCursor_EmptyString(t *testing.T) {
	// Empty string is invalid base64 (no padding).
	_, _, err := decodeCursor("")
	// The zero-length string decodes to zero bytes, which then fails JSON
	// unmarshal into cursor struct (no `t` or `id` fields).  We accept either
	// a decode error or a zero-value cursor — but the function must not panic.
	_ = err // may or may not error; main goal is no panic
}

func TestDecodeCursor_OutputIsURLSafe(t *testing.T) {
	// Encoded cursor must be URL-safe (no +, /, unpadded is fine for query
	// params since URLEncoding uses - and _).
	id := uuid.New()
	ts := time.Now().UTC()
	encoded := encodeCursor(ts, id)
	if strings.ContainsAny(encoded, "+/") {
		t.Errorf("encodeCursor produced non-URL-safe characters in %q", encoded)
	}
}

func TestDecodeCursor_MultipleRoundTrips(t *testing.T) {
	// Verifies deterministic encoding for consistent pagination tokens.
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	e1 := encodeCursor(ts, id)
	e2 := encodeCursor(ts, id)
	if e1 != e2 {
		t.Errorf("encodeCursor is non-deterministic: %q != %q", e1, e2)
	}

	_, _, err := decodeCursor(e1)
	if err != nil {
		t.Fatalf("second round-trip failed: %v", err)
	}
}

// =========================================================================
// textPtr
// =========================================================================

func TestTextPtr_Valid(t *testing.T) {
	pt := pgtype.Text{String: "hello", Valid: true}
	got := textPtr(pt)
	if got == nil {
		t.Fatal("expected non-nil pointer for valid pgtype.Text")
	}
	if *got != "hello" {
		t.Errorf("got %q, want %q", *got, "hello")
	}
}

func TestTextPtr_Invalid(t *testing.T) {
	pt := pgtype.Text{Valid: false}
	if got := textPtr(pt); got != nil {
		t.Errorf("expected nil for invalid pgtype.Text, got %q", *got)
	}
}

func TestTextPtr_EmptyString(t *testing.T) {
	// An empty but valid Text should return a pointer to "".
	pt := pgtype.Text{String: "", Valid: true}
	got := textPtr(pt)
	if got == nil {
		t.Fatal("expected non-nil pointer for valid empty pgtype.Text")
	}
	if *got != "" {
		t.Errorf("got %q, want empty string", *got)
	}
}

// =========================================================================
// uuidPtr
// =========================================================================

func TestUUIDPtr_Valid(t *testing.T) {
	id := uuid.MustParse("12345678-1234-1234-1234-123456789abc")
	nu := uuid.NullUUID{UUID: id, Valid: true}
	got := uuidPtr(nu)
	if got == nil {
		t.Fatal("expected non-nil pointer for valid NullUUID")
	}
	if *got != id.String() {
		t.Errorf("got %q, want %q", *got, id.String())
	}
}

func TestUUIDPtr_Invalid(t *testing.T) {
	nu := uuid.NullUUID{Valid: false}
	if got := uuidPtr(nu); got != nil {
		t.Errorf("expected nil for invalid NullUUID, got %q", *got)
	}
}

func TestUUIDPtr_NilUUID(t *testing.T) {
	// Valid but zero UUID.
	nu := uuid.NullUUID{UUID: uuid.Nil, Valid: true}
	got := uuidPtr(nu)
	if got == nil {
		t.Fatal("expected non-nil pointer for valid NullUUID with nil UUID value")
	}
	if *got != uuid.Nil.String() {
		t.Errorf("got %q, want %q", *got, uuid.Nil.String())
	}
}

// =========================================================================
// datePtr
// =========================================================================

func TestDatePtr_Valid(t *testing.T) {
	ts := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	d := pgtype.Date{Time: ts, Valid: true}
	got := datePtr(d)
	if got == nil {
		t.Fatal("expected non-nil pointer for valid pgtype.Date")
	}
	if *got != "2024-03-15" {
		t.Errorf("got %q, want %q", *got, "2024-03-15")
	}
}

func TestDatePtr_Invalid(t *testing.T) {
	d := pgtype.Date{Valid: false}
	if got := datePtr(d); got != nil {
		t.Errorf("expected nil for invalid pgtype.Date, got %q", *got)
	}
}

func TestDatePtr_Format(t *testing.T) {
	cases := []struct {
		t    time.Time
		want string
	}{
		{time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), "2000-01-01"},
		{time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC), "2099-12-31"},
		{time.Date(2024, 11, 5, 0, 0, 0, 0, time.UTC), "2024-11-05"},
	}
	for _, tc := range cases {
		d := pgtype.Date{Time: tc.t, Valid: true}
		got := datePtr(d)
		if got == nil || *got != tc.want {
			t.Errorf("datePtr(%v) = %v, want %q", tc.t, got, tc.want)
		}
	}
}

// =========================================================================
// tsPtr
// =========================================================================

func TestTsPtr_Valid(t *testing.T) {
	ts := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	pt := pgtype.Timestamptz{Time: ts, Valid: true}
	got := tsPtr(pt)
	if got == nil {
		t.Fatal("expected non-nil pointer for valid Timestamptz")
	}
	if !got.Equal(ts) {
		t.Errorf("got %v, want %v", *got, ts)
	}
}

func TestTsPtr_Invalid(t *testing.T) {
	pt := pgtype.Timestamptz{Valid: false}
	if got := tsPtr(pt); got != nil {
		t.Errorf("expected nil for invalid Timestamptz, got %v", *got)
	}
}

// =========================================================================
// numPtr
// =========================================================================

func TestNumPtr_Valid(t *testing.T) {
	var n pgtype.Numeric
	if err := n.Scan("123.45"); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got := numPtr(n)
	if got == nil {
		t.Fatal("expected non-nil pointer for valid Numeric")
	}
	const want = 123.45
	if *got < want-0.001 || *got > want+0.001 {
		t.Errorf("got %v, want ~%v", *got, want)
	}
}

func TestNumPtr_Zero(t *testing.T) {
	var n pgtype.Numeric
	if err := n.Scan("0"); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got := numPtr(n)
	if got == nil {
		t.Fatal("expected non-nil pointer for zero Numeric")
	}
	if *got != 0 {
		t.Errorf("got %v, want 0", *got)
	}
}

func TestNumPtr_Invalid(t *testing.T) {
	n := pgtype.Numeric{Valid: false}
	if got := numPtr(n); got != nil {
		t.Errorf("expected nil for invalid Numeric, got %v", *got)
	}
}

func TestNumPtr_NegativeValue(t *testing.T) {
	var n pgtype.Numeric
	if err := n.Scan("-99.99"); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got := numPtr(n)
	if got == nil {
		t.Fatal("expected non-nil for negative Numeric")
	}
	if *got > -99 {
		t.Errorf("got %v, want ~-99.99", *got)
	}
}

// =========================================================================
// toText
// =========================================================================

func TestToText_NonNil(t *testing.T) {
	s := "world"
	got := toText(&s)
	if !got.Valid {
		t.Fatal("expected Valid=true for non-nil string")
	}
	if got.String != s {
		t.Errorf("got %q, want %q", got.String, s)
	}
}

func TestToText_Nil(t *testing.T) {
	got := toText(nil)
	if got.Valid {
		t.Fatal("expected Valid=false for nil string")
	}
}

func TestToText_EmptyString(t *testing.T) {
	s := ""
	got := toText(&s)
	if !got.Valid {
		t.Fatal("expected Valid=true for pointer to empty string")
	}
	if got.String != "" {
		t.Errorf("got %q, want empty string", got.String)
	}
}

func TestToText_RoundTrip(t *testing.T) {
	// toText → textPtr should be identity.
	s := "round trip value"
	got := textPtr(toText(&s))
	if got == nil || *got != s {
		t.Errorf("round-trip: got %v, want %q", got, s)
	}

	// nil round-trip.
	if textPtr(toText(nil)) != nil {
		t.Error("nil round-trip: expected nil")
	}
}

// =========================================================================
// toNullUUID
// =========================================================================

func TestToNullUUID_Valid(t *testing.T) {
	id := uuid.New()
	s := id.String()
	got := toNullUUID(&s)
	if !got.Valid {
		t.Fatal("expected Valid=true for valid UUID string")
	}
	if got.UUID != id {
		t.Errorf("got %v, want %v", got.UUID, id)
	}
}

func TestToNullUUID_Nil(t *testing.T) {
	got := toNullUUID(nil)
	if got.Valid {
		t.Fatal("expected Valid=false for nil string")
	}
}

func TestToNullUUID_InvalidUUID(t *testing.T) {
	s := "not-a-uuid"
	got := toNullUUID(&s)
	if got.Valid {
		t.Fatal("expected Valid=false for invalid UUID string")
	}
}

func TestToNullUUID_MalformedUUID(t *testing.T) {
	cases := []string{
		"",
		"12345",
		"zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",
		"11111111-2222-3333-4444",     // too short
		"11111111-2222-3333-4444-55555555555555", // too long
	}
	for _, s := range cases {
		sc := s
		got := toNullUUID(&sc)
		if got.Valid {
			t.Errorf("toNullUUID(%q): expected Valid=false for malformed UUID", s)
		}
	}
}

func TestToNullUUID_RoundTrip(t *testing.T) {
	id := uuid.New()
	s := id.String()
	got := uuidPtr(toNullUUID(&s))
	if got == nil || *got != s {
		t.Errorf("round-trip: got %v, want %q", got, s)
	}

	// nil round-trip.
	if uuidPtr(toNullUUID(nil)) != nil {
		t.Error("nil round-trip: expected nil")
	}
}

// =========================================================================
// toNumeric
// =========================================================================

func TestToNumeric_NonNil(t *testing.T) {
	f := 42.5
	got := toNumeric(&f)
	if !got.Valid {
		t.Fatal("expected Valid=true for non-nil float64")
	}
	// Round-trip through numPtr.
	result := numPtr(got)
	if result == nil {
		t.Fatal("numPtr of toNumeric returned nil")
	}
	if *result < 42.4 || *result > 42.6 {
		t.Errorf("got %v, want ~42.5", *result)
	}
}

func TestToNumeric_Nil(t *testing.T) {
	got := toNumeric(nil)
	if got.Valid {
		t.Fatal("expected Valid=false for nil float64")
	}
}

func TestToNumeric_Zero(t *testing.T) {
	f := 0.0
	got := toNumeric(&f)
	if !got.Valid {
		t.Fatal("expected Valid=true for zero float64")
	}
}

func TestToNumeric_RoundTrip(t *testing.T) {
	values := []float64{0, 1, -1, 999999.99, 0.001}
	for _, v := range values {
		vc := v
		n := toNumeric(&vc)
		if !n.Valid {
			t.Errorf("toNumeric(%v): expected Valid=true", v)
			continue
		}
		result := numPtr(n)
		if result == nil {
			t.Errorf("numPtr(toNumeric(%v)): got nil", v)
			continue
		}
		diff := *result - v
		if diff < -0.0001 || diff > 0.0001 {
			t.Errorf("round-trip(%v): got %v", v, *result)
		}
	}
}

// =========================================================================
// toDate
// =========================================================================

func TestToDate_Valid(t *testing.T) {
	s := "2024-07-04"
	got := toDate(&s)
	if !got.Valid {
		t.Fatal("expected Valid=true for valid date string")
	}
	if got.Time.Year() != 2024 || got.Time.Month() != 7 || got.Time.Day() != 4 {
		t.Errorf("got %v, want 2024-07-04", got.Time)
	}
}

func TestToDate_Nil(t *testing.T) {
	got := toDate(nil)
	if got.Valid {
		t.Fatal("expected Valid=false for nil string")
	}
}

func TestToDate_InvalidFormat(t *testing.T) {
	cases := []string{
		"not-a-date",
		"07/04/2024",
		"2024/07/04",
		"20240704",
		"",
		"2024-13-01", // invalid month
		"2024-00-01", // invalid month zero
	}
	for _, s := range cases {
		sc := s
		got := toDate(&sc)
		if got.Valid {
			t.Errorf("toDate(%q): expected Valid=false for invalid date", s)
		}
	}
}

func TestToDate_RoundTrip(t *testing.T) {
	s := "2025-12-25"
	got := datePtr(toDate(&s))
	if got == nil || *got != s {
		t.Errorf("round-trip: got %v, want %q", got, s)
	}
}

// =========================================================================
// writeJSON
// =========================================================================

func TestWriteJSON_StatusAndContentType(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

func TestWriteJSON_BodyIsValidJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, payload{Name: "Alice", Age: 30})

	if w.Code != http.StatusCreated {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusCreated)
	}
	var got payload
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if got.Name != "Alice" || got.Age != 30 {
		t.Errorf("got %+v, want {Alice 30}", got)
	}
}

func TestWriteJSON_NilValue(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, nil)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	// nil encodes to "null\n" — must be valid JSON.
	body := strings.TrimSpace(w.Body.String())
	if body != "null" {
		t.Errorf("body: got %q, want %q", body, "null")
	}
}

func TestWriteJSON_StatusCodes(t *testing.T) {
	cases := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNoContent,
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}
	for _, code := range cases {
		w := httptest.NewRecorder()
		writeJSON(w, code, struct{}{})
		if w.Code != code {
			t.Errorf("writeJSON(%d): got status %d", code, w.Code)
		}
	}
}

// =========================================================================
// writeErr
// =========================================================================

func TestWriteErr_Format(t *testing.T) {
	w := httptest.NewRecorder()
	writeErr(w, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if body["error"] != "something went wrong" {
		t.Errorf("error field: got %q, want %q", body["error"], "something went wrong")
	}
	// Must have exactly one key: "error".
	if len(body) != 1 {
		t.Errorf("body has %d keys, want 1: %v", len(body), body)
	}
}

func TestWriteErr_404(t *testing.T) {
	w := httptest.NewRecorder()
	writeErr(w, http.StatusNotFound, "contact not found")

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNotFound)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("body decode: %v", err)
	}
	if body["error"] != "contact not found" {
		t.Errorf("error: got %q, want %q", body["error"], "contact not found")
	}
}

func TestWriteErr_EmptyMessage(t *testing.T) {
	w := httptest.NewRecorder()
	writeErr(w, http.StatusInternalServerError, "")

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("body decode: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("missing 'error' key in response body")
	}
}

// =========================================================================
// decodeBody
// =========================================================================

func TestDecodeBody_ValidJSON(t *testing.T) {
	type req struct {
		Name string `json:"name"`
	}
	body := `{"name":"test"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var dst req
	ok := decodeBody(w, r, &dst)
	if !ok {
		t.Fatal("decodeBody returned false for valid JSON")
	}
	if dst.Name != "test" {
		t.Errorf("Name: got %q, want %q", dst.Name, "test")
	}
	// Recorder defaults to 200; decodeBody must not have written a status.
	if w.Code != http.StatusOK {
		t.Errorf("decodeBody wrote status %d on success, want 200", w.Code)
	}
}

func TestDecodeBody_InvalidJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{invalid json"))
	w := httptest.NewRecorder()

	var dst struct{ Name string }
	ok := decodeBody(w, r, &dst)
	if ok {
		t.Fatal("decodeBody returned true for invalid JSON")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("response body decode: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected non-empty error field in response body")
	}
}

func TestDecodeBody_EmptyBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	w := httptest.NewRecorder()

	var dst struct{ Name string }
	ok := decodeBody(w, r, &dst)
	if ok {
		t.Fatal("decodeBody returned true for empty body (EOF)")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDecodeBody_BodyTooLarge(t *testing.T) {
	// Generate a body larger than maxBodySize (1 MiB).
	// The JSON wraps the large value so it looks structurally valid to the
	// decoder until it hits the size limit.
	bigValue := strings.Repeat("x", int(maxBodySize)+100)
	bigJSON := `{"name":"` + bigValue + `"}`
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(bigJSON))
	w := httptest.NewRecorder()

	var dst struct{ Name string }
	ok := decodeBody(w, r, &dst)
	if ok {
		t.Fatal("decodeBody returned true for body that exceeds maxBodySize")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDecodeBody_WrongType(t *testing.T) {
	// Sending an array where an object is expected — JSON type mismatch.
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`[1,2,3]`))
	w := httptest.NewRecorder()

	var dst struct{ Name string }
	ok := decodeBody(w, r, &dst)
	if ok {
		t.Fatal("decodeBody returned true for type mismatch")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDecodeBody_ExtraFieldsIgnored(t *testing.T) {
	// json.Decoder ignores unknown fields by default.
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"Alice","unknown_field":"ignored"}`))
	w := httptest.NewRecorder()

	var dst struct {
		Name string `json:"name"`
	}
	ok := decodeBody(w, r, &dst)
	if !ok {
		t.Fatal("decodeBody returned false for JSON with extra fields")
	}
	if dst.Name != "Alice" {
		t.Errorf("Name: got %q, want %q", dst.Name, "Alice")
	}
}

// =========================================================================
// parseID
// =========================================================================

// newRequestWithChiID creates an httptest.Request with the chi route context
// populated so that chi.URLParam(r, "id") returns idVal.
func newRequestWithChiID(idVal string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/items/"+idVal, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", idVal)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestParseID_ValidUUID(t *testing.T) {
	id := uuid.New()
	r := newRequestWithChiID(id.String())
	w := httptest.NewRecorder()

	got, ok := parseID(w, r)
	if !ok {
		t.Fatalf("parseID returned false for valid UUID %v; response: %s", id, w.Body.String())
	}
	if got != id {
		t.Errorf("got %v, want %v", got, id)
	}
	// httptest.Recorder starts at 200; parseID must not have called WriteHeader.
	if w.Code != http.StatusOK {
		t.Errorf("parseID wrote status %d, want 200 (no error)", w.Code)
	}
}

func TestParseID_InvalidUUID(t *testing.T) {
	cases := []string{
		"not-a-uuid",
		"12345",
		"",
		"zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",
		"11111111-2222-3333-4444",
	}
	for _, idVal := range cases {
		r := newRequestWithChiID(idVal)
		w := httptest.NewRecorder()

		_, ok := parseID(w, r)
		if ok {
			t.Errorf("parseID(%q): returned true, expected false", idVal)
			continue
		}
		if w.Code != http.StatusBadRequest {
			t.Errorf("parseID(%q): status %d, want %d", idVal, w.Code, http.StatusBadRequest)
		}
		var body map[string]string
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Errorf("parseID(%q): response body is not valid JSON: %v", idVal, err)
			continue
		}
		if body["error"] == "" {
			t.Errorf("parseID(%q): expected non-empty error field", idVal)
		}
	}
}

func TestParseID_NilUUID(t *testing.T) {
	// uuid.Nil (all zeros) is a valid UUID format and parseID must accept it.
	r := newRequestWithChiID(uuid.Nil.String())
	w := httptest.NewRecorder()

	got, ok := parseID(w, r)
	if !ok {
		t.Fatalf("parseID returned false for uuid.Nil: %s", w.Body.String())
	}
	if got != uuid.Nil {
		t.Errorf("got %v, want uuid.Nil", got)
	}
}
