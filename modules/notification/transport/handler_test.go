package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	"github.com/sujanto-gaws/kopiochi/modules/notification/application"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// What these tests pin, in order of how much they matter:
//
//  1. Every route acts on the Principal and on nothing else. This is the first
//     id-bearing route table in the repository — modules/user closed E16 by
//     deleting its id-bearing routes, and marking one notification read cannot
//     be expressed without naming it — so the property that stood in for an
//     ownership check there has to be asserted directly here.
//  2. A cross-user id is answered exactly as an absent one. Not nearly: the
//     same status, the same headers and the same body, so the id space is not
//     enumerable through the difference.
//  3. Everything else: projection, paging, the 422s, and that a failing store
//     leaks nothing.
//
// Nothing here mints a token. testsupport.FakeAuth supplies the caller
// directly, because what the handlers depend on is an authn.Principal in the
// context, not RS256. The one test that needs the real middleware — 401 without
// a credential — lives in cmd/api, which is the only package allowed to import
// both this module and identity's.

var (
	testSubject = uuid.MustParse("3f1b8a54-2c9e-4d77-9a1e-2b6c0d5e8f41")

	// testCreatedAt is fixed and carries microseconds, which is the resolution
	// Postgres stores: a cursor that round-trips at second precision would pass
	// a test written against time.Now().Truncate(time.Second) and drop rows in
	// production.
	testCreatedAt = time.Date(2026, 8, 30, 9, 14, 15, 123456000, time.UTC)
)

// fakeService records what it was handed. The recording is the assertion: the
// interface takes the caller first and nothing in a request can name a
// different one, so what is left to verify is that the id which arrives is the
// authenticated subject.
type fakeService struct {
	calls      int
	sawCaller  uuid.UUID
	sawFilter  domain.ListFilter
	sawID      uuid.UUID
	sawUpdates []application.PreferenceUpdate

	list           []application.NotificationView
	listErr        error
	markReadErr    error
	markedAllRead  int
	markAllReadErr error
	prefs          []application.PreferenceView
	prefsErr       error
	updateErr      error
}

func (f *fakeService) ListForUser(_ context.Context, userID uuid.UUID, filter domain.ListFilter) ([]application.NotificationView, error) {
	f.calls++
	f.sawCaller = userID
	f.sawFilter = filter
	return f.list, f.listErr
}

func (f *fakeService) MarkRead(_ context.Context, userID, id uuid.UUID) error {
	f.calls++
	f.sawCaller = userID
	f.sawID = id
	return f.markReadErr
}

func (f *fakeService) MarkAllRead(_ context.Context, userID uuid.UUID) (int, error) {
	f.calls++
	f.sawCaller = userID
	return f.markedAllRead, f.markAllReadErr
}

func (f *fakeService) GetPreferences(_ context.Context, userID uuid.UUID) ([]application.PreferenceView, error) {
	f.calls++
	f.sawCaller = userID
	return f.prefs, f.prefsErr
}

func (f *fakeService) UpdatePreferences(_ context.Context, userID uuid.UUID, updates []application.PreferenceUpdate) ([]application.PreferenceView, error) {
	f.calls++
	f.sawCaller = userID
	f.sawUpdates = updates
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.prefs, nil
}

// router mounts the real route table behind a middleware that authenticates
// every request as subject.
func router(svc Service, subject string) http.Handler {
	r := chi.NewRouter()
	NewHandler(svc, testsupport.FakeAuth(subject)).Routes(r)
	return r
}

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("response is not the expected JSON: %v (%q)", err, rec.Body)
	}
	return v
}

// everyRoute is the whole surface, listed once so a test that must cover all of
// it cannot silently cover four fifths. A new route that is not added here is a
// route no scoping test walks.
var everyRoute = []struct {
	name   string
	method string
	path   string
}{
	{"list", http.MethodGet, "/notifications"},
	{"mark one read", http.MethodPost, "/notifications/" + uuid.Nil.String() + "/read"},
	{"mark all read", http.MethodPost, "/notifications/read-all"},
	{"get preferences", http.MethodGet, "/notifications/preferences"},
	{"put preferences", http.MethodPut, "/notifications/preferences"},
}

// TestEveryHandlerActsOnThePrincipalAndNothingElse is R5 at the transport
// boundary: the recipient the service is scoped by comes from the credential,
// and there is no request a client can write that changes it.
func TestEveryHandlerActsOnThePrincipalAndNothingElse(t *testing.T) {
	for _, tc := range everyRoute {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{}

			rec := doJSON(t, router(svc, testSubject.String()), tc.method, tc.path, `{"preferences":[]}`)
			if rec.Code >= 500 {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body)
			}
			if svc.calls != 1 {
				t.Fatalf("the service was called %d times, want 1", svc.calls)
			}
			if svc.sawCaller != testSubject {
				t.Errorf("service was given %v, want the authenticated subject %v",
					svc.sawCaller, testSubject)
			}
		})
	}
}

// TestUnauthenticatedRequestsNeverReachTheService: the counter, not the status
// code, is the assertion. A 401 with the handler having already run is a
// different and worse thing than a 401 instead of it.
func TestUnauthenticatedRequestsNeverReachTheService(t *testing.T) {
	for _, tc := range everyRoute {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{}

			r := chi.NewRouter()
			// A middleware that authenticates nobody, standing in for a
			// request with no credentials.
			NewHandler(svc, func(http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				})
			}).Routes(r)

			rec := doJSON(t, r, tc.method, tc.path, `{"preferences":[]}`)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if svc.calls != 0 {
				t.Errorf("the service ran %d times for an unauthenticated request", svc.calls)
			}
		})
	}
}

// TestAMalformedSubjectIsTheCanonical401 is caller()'s one interesting branch:
// the credential verified, but its subject is not an id this module can act
// for.
//
// It must be the canonical 401 and not an invention of this package — same
// challenge header, same body, same detail as every other rejection in the
// service — because a rejection that looks different is a rejection a client
// can tell apart, and the whole point of httpx.Unauthorized is that none of
// them can be.
func TestAMalformedSubjectIsTheCanonical401(t *testing.T) {
	for _, tc := range everyRoute {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{}

			rec := doJSON(t, router(svc, "not-a-uuid"), tc.method, tc.path, `{"preferences":[]}`)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != `Bearer realm="api"` {
				t.Errorf("WWW-Authenticate = %q, want the canonical challenge", got)
			}
			if got := rec.Header().Get("Content-Type"); got != httpx.ProblemContentType {
				t.Errorf("Content-Type = %q, want %q", got, httpx.ProblemContentType)
			}
			p := decode[httpx.Problem](t, rec)
			if p.Detail != "authentication required" {
				t.Errorf("detail = %q; every authentication failure says the same thing", p.Detail)
			}
			if svc.calls != 0 {
				t.Errorf("the service ran %d times for a caller with no usable id", svc.calls)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The property this task exists for.
// ---------------------------------------------------------------------------

// TestMarkReadCannotDistinguishAForeignIdFromAnAbsentOne is E16-3 as it was
// originally scoped, on the module that cannot dodge it by deleting the route.
//
// The service reports domain.ErrNotFound for a row that does not exist and for
// a row that belongs to somebody else — the repository's UPDATE has the
// recipient in its WHERE clause and counts zero either way, so the two are not
// merely reported alike, they are indistinguishable before the handler is
// reached. This asserts the property survives transport: same status, same
// headers, same body.
//
// A third probe is in the table on purpose. An id that is not a uuid takes a
// different code path (it never reaches the service) and must still come back
// with the identical answer, because "one code path" is easy to claim and this
// is the case that would break it.
//
// instance is the one member that varies, and it is the path the client itself
// wrote — it carries no information the client did not already have. Everything
// else is compared byte for byte.
func TestMarkReadCannotDistinguishAForeignIdFromAnAbsentOne(t *testing.T) {
	foreign := uuid.MustParse("b0a1f4c2-7d3e-4a58-9c61-0e2d4f6a8b10")
	absent := uuid.MustParse("c1b2e5d3-8e4f-4b69-8d72-1f3e5a7b9c21")

	probes := []struct {
		name string
		path string
		// svcErr is what MarkRead reports. An unparsable id never gets that
		// far, which is exactly why it is here.
		svcErr    error
		wantCalls int
	}{
		{"somebody else's notification", "/notifications/" + foreign.String() + "/read", domain.ErrNotFound, 1},
		{"a notification that does not exist", "/notifications/" + absent.String() + "/read", domain.ErrNotFound, 1},
		{"an id that is not a uuid", "/notifications/not-a-uuid/read", nil, 0},
	}

	type answer struct {
		status  int
		header  http.Header
		problem httpx.Problem
		body    string
	}

	answers := make([]answer, 0, len(probes))
	for _, p := range probes {
		t.Run(p.name, func(t *testing.T) {
			svc := &fakeService{markReadErr: p.svcErr}

			rec := do(t, router(svc, testSubject.String()), http.MethodPost, p.path)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
			}
			if svc.calls != p.wantCalls {
				t.Errorf("the service was called %d times, want %d", svc.calls, p.wantCalls)
			}

			problem := decode[httpx.Problem](t, rec)
			if problem.Instance != p.path {
				t.Errorf("instance = %q, want the request's own path %q", problem.Instance, p.path)
			}
			if strings.Contains(problem.Detail, foreign.String()) || strings.Contains(problem.Detail, absent.String()) {
				t.Errorf("the detail echoes the id back: %q", problem.Detail)
			}

			// Blanked because it is the one member allowed to differ; the
			// assertion above is what holds it to the request's own path.
			problem.Instance = ""

			answers = append(answers, answer{
				status:  rec.Code,
				header:  rec.Header().Clone(),
				problem: problem,
				// The id substituted out, so the remaining bytes are
				// comparable across probes that necessarily used different
				// paths.
				body: strings.ReplaceAll(strings.ReplaceAll(
					strings.ReplaceAll(rec.Body.String(), foreign.String(), "{id}"),
					absent.String(), "{id}"), "not-a-uuid", "{id}"),
			})
		})
	}

	if len(answers) != len(probes) {
		t.Fatal("a probe failed before it could be compared; the comparison below proves nothing")
	}

	first := answers[0]
	for i, got := range answers[1:] {
		name := probes[i+1].name
		if got.status != first.status {
			t.Errorf("%s answered %d, and %s answered %d: the difference enumerates the id space",
				name, got.status, probes[0].name, first.status)
		}
		if fmt.Sprint(got.header) != fmt.Sprint(first.header) {
			t.Errorf("%s answered with headers %v, and %s with %v", name, got.header, probes[0].name, first.header)
		}
		if got.problem != first.problem {
			t.Errorf("%s answered %+v, and %s answered %+v", name, got.problem, probes[0].name, first.problem)
		}
		if got.body != first.body {
			t.Errorf("%s answered body %q, and %s answered %q", name, got.body, probes[0].name, first.body)
		}
	}
}

// TestMarkReadPassesTheCallerAsTheRecipientAndTheURLAsTheRow: the two ids the
// handler juggles, each going where it belongs. Swapping them is the shape of
// the bug this whole file is about, and it would otherwise typecheck.
func TestMarkReadPassesTheCallerAsTheRecipientAndTheURLAsTheRow(t *testing.T) {
	row := uuid.MustParse("d2c3f6e4-9f50-4c7a-9e83-2a4b6c8d0e32")
	svc := &fakeService{}

	rec := do(t, router(svc, testSubject.String()), http.MethodPost, "/notifications/"+row.String()+"/read")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 carries a body: %q", rec.Body)
	}
	if svc.sawCaller != testSubject {
		t.Errorf("recipient = %v, want the authenticated subject %v", svc.sawCaller, testSubject)
	}
	if svc.sawID != row {
		t.Errorf("id = %v, want the one in the URL %v", svc.sawID, row)
	}
}

// ---------------------------------------------------------------------------
// The mailbox read model.
// ---------------------------------------------------------------------------

func aView(id uuid.UUID, createdAt time.Time, readAt *time.Time) application.NotificationView {
	return application.NotificationView{
		ID:          id,
		Category:    domain.CategorySecurity,
		TemplateKey: "security.password_changed",
		Payload:     map[string]any{"ChangedAt": "30 August 2026 at 09:14 UTC"},
		CreatedAt:   createdAt,
		ReadAt:      readAt,
	}
}

func TestListProjectsTheMailboxOntoTheWire(t *testing.T) {
	id := uuid.MustParse("e3d4a7f5-0a61-4d8b-8f94-3b5c7d9e1f43")
	read := testCreatedAt.Add(time.Minute)
	svc := &fakeService{list: []application.NotificationView{
		aView(id, testCreatedAt, nil),
		aView(uuid.New(), testCreatedAt.Add(-time.Hour), &read),
	}}

	rec := do(t, router(svc, testSubject.String()), http.MethodGet, "/notifications")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	got := decode[ListNotificationsResponse](t, rec)
	if len(got.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(got.Items))
	}
	if got.Items[0].ID != id.String() {
		t.Errorf("id = %q, want %q", got.Items[0].ID, id)
	}
	if !got.Items[0].CreatedAt.Equal(testCreatedAt) {
		t.Errorf("created_at = %v, want %v", got.Items[0].CreatedAt, testCreatedAt)
	}
	if got.Items[0].ReadAt != nil {
		t.Errorf("read_at = %v, want null for an unread row", got.Items[0].ReadAt)
	}
	if got.Items[1].ReadAt == nil || !got.Items[1].ReadAt.Equal(read) {
		t.Errorf("read_at = %v, want %v", got.Items[1].ReadAt, read)
	}

	// Delivery state is operational and never leaves the service: LastError in
	// particular carries SMTP responses and internal host names.
	raw := rec.Body.String()
	for _, forbidden := range []string{"status", "attempts", "last_error", "recipient", "channel"} {
		if strings.Contains(raw, `"`+forbidden+`"`) {
			t.Errorf("the mailbox response carries %q, which is not the user's business: %s", forbidden, raw)
		}
	}
}

// TestAnEmptyMailboxIsAnEmptyArray: a client rendering a list should not have
// to tell "no notifications" from "the field was omitted".
func TestAnEmptyMailboxIsAnEmptyArray(t *testing.T) {
	rec := do(t, router(&fakeService{}, testSubject.String()), http.MethodGet, "/notifications")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("an empty page is not []: %s", rec.Body)
	}
}

func TestListPassesTheQueryThroughAsAFilter(t *testing.T) {
	before := domain.Cursor{CreatedAt: testCreatedAt, ID: uuid.MustParse("f4e5b8a6-1b72-4e9c-8a05-4c6d8e0f2a54")}

	svc := &fakeService{}
	path := "/notifications?unread=true&limit=7&cursor=" + encodeCursor(before)

	rec := do(t, router(svc, testSubject.String()), http.MethodGet, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if !svc.sawFilter.UnreadOnly {
		t.Error("unread=true did not reach the filter")
	}
	if svc.sawFilter.Limit != 7 {
		t.Errorf("limit = %d, want 7", svc.sawFilter.Limit)
	}
	if svc.sawFilter.Before == nil {
		t.Fatal("the cursor did not reach the filter")
	}
	// Equal and not ==: a time that survived a JSON round trip is the same
	// instant, and asserting on the struct would fail on the monotonic-clock
	// and location fields instead.
	if !svc.sawFilter.Before.CreatedAt.Equal(before.CreatedAt) || svc.sawFilter.Before.ID != before.ID {
		t.Errorf("cursor = %+v, want %+v", *svc.sawFilter.Before, before)
	}
}

// TestListDefaultsAndCapsTheLimit: the two bounds come from the domain, and
// transport applies the same Normalize the repository will, because it needs
// the effective limit to decide whether the page it got back was full.
func TestListDefaultsAndCapsTheLimit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
		want  int
	}{
		{"unspecified", "", domain.DefaultListLimit},
		{"above the maximum is capped, not refused", "?limit=100000", domain.MaxListLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{}

			rec := do(t, router(svc, testSubject.String()), http.MethodGet, "/notifications"+tc.query)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
			}
			if svc.sawFilter.Limit != tc.want {
				t.Errorf("limit = %d, want %d", svc.sawFilter.Limit, tc.want)
			}
		})
	}
}

// TestListRefusesMalformedQueryParameters. Each of these is something the
// caller wrote, so each is named in the response — that is help, not
// disclosure. A limit of zero is refused rather than defaulted: Normalize reads
// non-positive as "unspecified", which is right for a caller who said nothing
// and wrong for one who said 0.
func TestListRefusesMalformedQueryParameters(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
	}{
		{"unread is not a boolean", "?unread=perhaps"},
		{"limit is not a number", "?limit=twenty"},
		{"limit is zero", "?limit=0"},
		{"limit is negative", "?limit=-1"},
		{"the cursor is not base64", "?cursor=!!!not-base64!!!"},
		{"the cursor is base64 of nothing useful", "?cursor=aGVsbG8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{}

			rec := do(t, router(svc, testSubject.String()), http.MethodGet, "/notifications"+tc.query)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != httpx.ProblemContentType {
				t.Errorf("Content-Type = %q, want %q", ct, httpx.ProblemContentType)
			}
			if svc.calls != 0 {
				t.Errorf("a request that could not be parsed reached the service %d times", svc.calls)
			}
		})
	}
}

// TestNextCursorAppearsOnlyOnAFullPageAndSeeksFromTheLastRow.
//
// A full page means there may be more, and that is all the query can know
// without fetching a row it was not asked for. So the next page may be empty —
// the honest answer for a keyset scan, and cheaper than over-fetching by one on
// every request to avoid one empty request at the end.
func TestNextCursorAppearsOnlyOnAFullPageAndSeeksFromTheLastRow(t *testing.T) {
	lastID := uuid.MustParse("a5f6c9b7-2c83-4f0d-9b16-5d7e9f0a3b65")
	lastCreatedAt := testCreatedAt.Add(-time.Hour)

	full := make([]application.NotificationView, 0, 3)
	full = append(full, aView(uuid.New(), testCreatedAt, nil))
	full = append(full, aView(uuid.New(), testCreatedAt.Add(-time.Minute), nil))
	full = append(full, aView(lastID, lastCreatedAt, nil))

	t.Run("a full page carries one", func(t *testing.T) {
		svc := &fakeService{list: full}

		rec := do(t, router(svc, testSubject.String()), http.MethodGet, "/notifications?limit=3")
		got := decode[ListNotificationsResponse](t, rec)
		if got.NextCursor == "" {
			t.Fatal("a full page carries no next_cursor; the caller cannot page")
		}

		// Fed straight back, the way a client would.
		next := &fakeService{}
		rec = do(t, router(next, testSubject.String()), http.MethodGet, "/notifications?limit=3&cursor="+got.NextCursor)
		if rec.Code != http.StatusOK {
			t.Fatalf("the cursor this service issued was refused: %d %s", rec.Code, rec.Body)
		}
		if next.sawFilter.Before == nil {
			t.Fatal("the returned cursor did not seek")
		}
		if next.sawFilter.Before.ID != lastID || !next.sawFilter.Before.CreatedAt.Equal(lastCreatedAt) {
			t.Errorf("seeks from %+v, want the last row of the previous page (%v, %v)",
				*next.sawFilter.Before, lastCreatedAt, lastID)
		}
	})

	t.Run("a short page does not", func(t *testing.T) {
		svc := &fakeService{list: full[:2]}

		rec := do(t, router(svc, testSubject.String()), http.MethodGet, "/notifications?limit=3")
		got := decode[ListNotificationsResponse](t, rec)
		if got.NextCursor != "" {
			t.Errorf("next_cursor = %q on a page that was not full", got.NextCursor)
		}
		if strings.Contains(rec.Body.String(), "next_cursor") {
			t.Errorf("next_cursor is present but empty; it should be omitted: %s", rec.Body)
		}
	})
}

// TestTheCursorIsOpaque: not a promise to clients, a constraint on us. A cursor
// that reads as "timestamp,uuid" is a cursor clients will parse, and the day
// the mailbox gains a second sort key every one of them breaks.
func TestTheCursorIsOpaque(t *testing.T) {
	id := uuid.MustParse("b6a7d0c8-3d94-4a1e-8c27-6e8f0a1b4c76")
	got := encodeCursor(domain.Cursor{CreatedAt: testCreatedAt, ID: id})

	if strings.Contains(got, id.String()) {
		t.Errorf("the cursor spells the id out: %q", got)
	}
	if strings.Contains(got, "2026") {
		t.Errorf("the cursor spells the timestamp out: %q", got)
	}
}

func TestMarkAllReadReportsHowManyRowsChanged(t *testing.T) {
	svc := &fakeService{markedAllRead: 4}

	rec := do(t, router(svc, testSubject.String()), http.MethodPost, "/notifications/read-all")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := decode[MarkAllReadResponse](t, rec); got.MarkedRead != 4 {
		t.Errorf("marked_read = %d, want 4", got.MarkedRead)
	}
}

// ---------------------------------------------------------------------------
// Preferences.
// ---------------------------------------------------------------------------

func matrix() []application.PreferenceView {
	out := make([]application.PreferenceView, 0, 9)
	for _, ch := range domain.Channels() {
		for _, cat := range domain.Categories() {
			out = append(out, application.PreferenceView{Channel: ch, Category: cat, Enabled: true})
		}
	}
	return out
}

func TestGetPreferencesReturnsTheWholeMatrix(t *testing.T) {
	svc := &fakeService{prefs: matrix()}

	rec := do(t, router(svc, testSubject.String()), http.MethodGet, "/notifications/preferences")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	got := decode[PreferencesResponse](t, rec)
	if want := len(domain.Channels()) * len(domain.Categories()); len(got.Preferences) != want {
		t.Fatalf("matrix has %d cells, want %d — a caller should never have to know what an absent pair means",
			len(got.Preferences), want)
	}
	if got.Preferences[0].Channel != string(domain.ChannelEmail) {
		t.Errorf("channel = %q, want %q", got.Preferences[0].Channel, domain.ChannelEmail)
	}
}

func TestUpdatePreferencesAppliesTheChangesAndReturnsTheEffectiveMatrix(t *testing.T) {
	svc := &fakeService{prefs: matrix()}

	rec := doJSON(t, router(svc, testSubject.String()), http.MethodPut, "/notifications/preferences",
		`{"preferences":[{"channel":"inapp","category":"system","enabled":false}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if len(svc.sawUpdates) != 1 {
		t.Fatalf("updates = %d, want 1", len(svc.sawUpdates))
	}
	want := application.PreferenceUpdate{Channel: domain.ChannelInApp, Category: domain.CategorySystem, Enabled: false}
	if svc.sawUpdates[0] != want {
		t.Errorf("update = %+v, want %+v", svc.sawUpdates[0], want)
	}
	if got := decode[PreferencesResponse](t, rec); len(got.Preferences) == 0 {
		t.Error("the response carries no matrix; a caller cannot see what its change did")
	}
}

// TestUpdatePreferencesRefusesAProtectedPairWith422 is the blueprint's one
// named error case. It is 422 and not 400 because the body parsed — what is
// wrong with it is what it means.
func TestUpdatePreferencesRefusesAProtectedPairWith422(t *testing.T) {
	svc := &fakeService{updateErr: fmt.Errorf("save preferences: %w", domain.ErrPreferenceProtected)}

	rec := doJSON(t, router(svc, testSubject.String()), http.MethodPut, "/notifications/preferences",
		`{"preferences":[{"channel":"email","category":"security","enabled":false}]}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != httpx.ProblemContentType {
		t.Errorf("Content-Type = %q, want %q", ct, httpx.ProblemContentType)
	}
	p := decode[httpx.Problem](t, rec)
	if p.Type != "preference_protected" {
		t.Errorf("type = %q; a client cannot tell this from an unknown channel", p.Type)
	}
	if p.Detail == "" {
		t.Error("the refusal explains nothing; this is the one a user has to be shown")
	}
}

func TestUpdatePreferencesRefusesAnUnknownPairWith422(t *testing.T) {
	svc := &fakeService{updateErr: fmt.Errorf("%w: unknown channel %q", domain.ErrInvalidPreference, "carrier-pigeon")}

	rec := doJSON(t, router(svc, testSubject.String()), http.MethodPut, "/notifications/preferences",
		`{"preferences":[{"channel":"carrier-pigeon","category":"system","enabled":false}]}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body)
	}
	if p := decode[httpx.Problem](t, rec); p.Type != "invalid_preference" {
		t.Errorf("type = %q, want invalid_preference", p.Type)
	}
	// The domain names the offending value in its error; the response must not
	// echo it back, because nothing downstream escaped it.
	if strings.Contains(rec.Body.String(), "carrier-pigeon") {
		t.Errorf("the response reflects the caller's input back: %s", rec.Body)
	}
}

func TestUpdatePreferencesRefusesABodyItCannotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"not JSON at all", `{"preferences":`},
		{"a member with no decision", `{"preferences":[{"channel":"inapp","category":"system"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{}

			rec := doJSON(t, router(svc, testSubject.String()), http.MethodPut, "/notifications/preferences", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
			if svc.calls != 0 {
				t.Errorf("a body that could not be read reached the service %d times", svc.calls)
			}
		})
	}
}

// TestAnEmptyUpdateIsAReadOfTheMatrix. Not an error: the repository writes an
// empty slice as a no-op, and a client that PUTs "nothing to change" gets the
// current effective matrix, which is what it asked about.
func TestAnEmptyUpdateIsAReadOfTheMatrix(t *testing.T) {
	svc := &fakeService{prefs: matrix()}

	rec := doJSON(t, router(svc, testSubject.String()), http.MethodPut, "/notifications/preferences",
		`{"preferences":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if len(svc.sawUpdates) != 0 {
		t.Errorf("updates = %d, want none", len(svc.sawUpdates))
	}
}

// ---------------------------------------------------------------------------
// Failure.
// ---------------------------------------------------------------------------

// TestStoreFailuresAre500AndLeakNothing: an unreachable database must not be
// reported as "you have no notifications" — that would tell a caller their mail
// is gone during an outage — and the body must not carry the underlying error,
// which names hosts and drivers.
func TestStoreFailuresAre500AndLeakNothing(t *testing.T) {
	boom := errors.New("dial tcp 10.0.0.5:5432: connection refused")

	for _, tc := range []struct {
		name   string
		method string
		path   string
		svc    *fakeService
	}{
		{"list", http.MethodGet, "/notifications", &fakeService{listErr: boom}},
		{"mark one read", http.MethodPost, "/notifications/" + uuid.Nil.String() + "/read", &fakeService{markReadErr: boom}},
		{"mark all read", http.MethodPost, "/notifications/read-all", &fakeService{markAllReadErr: boom}},
		{"get preferences", http.MethodGet, "/notifications/preferences", &fakeService{prefsErr: boom}},
		{"put preferences", http.MethodPut, "/notifications/preferences", &fakeService{updateErr: boom}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, router(tc.svc, testSubject.String()), tc.method, tc.path, `{"preferences":[]}`)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != httpx.ProblemContentType {
				t.Errorf("Content-Type = %q, want %q", ct, httpx.ProblemContentType)
			}
			if strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Errorf("the response leaked the underlying error: %s", rec.Body)
			}
		})
	}
}

// A store failure on MarkRead must be a 500 and NOT the 404 the same handler
// emits for a missing row. Collapsing them would tell a caller their
// notification is gone whenever the database is unreachable — and would make
// the not-found answer, whose whole job is to mean one thing, mean two.
func TestMarkReadDoesNotReportAnOutageAsNotFound(t *testing.T) {
	svc := &fakeService{markReadErr: errors.New("connection reset by peer")}

	rec := do(t, router(svc, testSubject.String()), http.MethodPost,
		"/notifications/"+uuid.New().String()+"/read")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
