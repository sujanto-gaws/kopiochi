package transport

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	application "github.com/sujanto-gaws/kopiochi/modules/user/application"
	domain "github.com/sujanto-gaws/kopiochi/modules/user/domain"
)

// This file is a camera, not a specification.
//
// It records — byte for byte — what the four /api/v1/users routes answer today
// when an *authenticated* caller asks for a profile row that is not theirs, so
// that the ownership check arriving with E16 is a measured change rather than a
// guessed one. The goldens it writes are committed on their own so the
// pre-change shapes survive in git history as the audit trail.
//
// Read this before you touch a golden here:
//
//   - The cross-user cases currently SUCCEED. A caller holding a valid token
//     for account A reads, overwrites and deletes account B's row and gets
//     200/200/204 for it. That is the defect (escalation E16, a confirmed
//     IDOR). It is recorded exactly as it is. Nothing in this file fixes it,
//     no golden here is annotated as "wrong", and a golden that looks alarming
//     is a golden doing its job.
//   - Nothing here asserts the cases agree with each other. They do not.
//   - The real application service is wired up, not a stub, over an in-memory
//     repository fake. A stubbed service would decide for itself whether a
//     cross-user id is refused, and the answer to that question is exactly
//     what is being captured.
//
// The case that matters most is the pair get_cross_user / get_not_found. Today
// they differ: 200 with B's row versus 404. That difference is an enumeration
// oracle — it tells an attacker which profile ids exist. E16's fix has to make
// the cross-user response byte-identical to the genuinely-not-found response,
// or the 404 is a 403 with extra steps and the oracle survives the fix. The two
// goldens below are the baseline against which that equality becomes checkable,
// and TestIDORIsPresentToday states the inequality in code so the fix cannot be
// absorbed by a silent `-update`.
//
// Regenerate with: go test ./modules/user/transport/... -run TestCurrentUserRouteShapes -update

// updateGolden rewrites the golden files instead of comparing against them.
// The spelling is the plain flag.Bool one established by
// modules/identity/transport/unauthorized_golden_test.go (task A1); anything
// added later should match it.
var updateGolden = flag.Bool("update", false,
	"rewrite modules/user/transport/testdata/golden/*.json to match current behavior")

// goldenDir is relative to this package, which is where `go test` runs.
const goldenDir = "testdata/golden"

// The two callers.
//
// Both are spelled as uuids because that is what an authenticated subject is in
// this repository: auth_users.id is a uuid. The profile ids in the request
// paths below are small integers because users.id is a BIGSERIAL. The two
// namespaces never meet — there is no auth_user_id column anywhere — and that
// is the root cause E16-ARCH identified: no ownership check can be written
// today because there is no value to compare.
//
// Which means the "owner" of a fixture row is a convention held by this file
// and by nothing in production. It is stated here so the follow-on refactor has
// a written record of the intent the schema could not express:
//
//	profile id 1  ->  subjectA
//	profile id 2  ->  subjectB
const (
	subjectA = "11111111-1111-4111-8111-111111111111"
	subjectB = "22222222-2222-4222-8222-222222222222"
)

// Fixture timestamps are fixed so the recorded bodies are byte-stable across
// runs. They are supplied by the repository fake, not observed from Postgres:
// this capture is about status, media type, body shape and who is allowed to
// see them, not about where created_at comes from.
var (
	fixtureCreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	fixtureUpdatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
)

// routeGolden is the recorded exchange.
//
// Status, ContentType and Body are the wire response — the three members the
// dispatch names, and the only ones the later byte-identity check between
// get_cross_user and get_not_found is about.
//
// Method, Path, CallerSubject and RequestBody are recorded alongside them so a
// human auditing the file a year from now can see *who asked for what* without
// cross-referencing the test source. StoreAfter records the repository contents
// after the request, because for PUT and DELETE the interesting damage is not
// in the response at all — a 204 looks innocent until you notice which row is
// gone.
//
// Body is a string rather than an embedded object on purpose: a body that is
// not JSON at all — or that is JSON with a trailing newline — must round-trip
// unchanged. Re-encoding it through a struct would normalize away precisely the
// details this capture exists to preserve.
type routeGolden struct {
	Method        string   `json:"method"`
	Path          string   `json:"path"`
	CallerSubject string   `json:"caller_subject"`
	RequestBody   string   `json:"request_body"`
	Status        int      `json:"status"`
	ContentType   string   `json:"content_type"`
	Body          string   `json:"body"`
	StoreAfter    []string `json:"store_after"`
}

// routeCase is one request made by one authenticated caller.
type routeCase struct {
	// name doubles as the golden file's stem: <name>.json.
	name string
	// note explains, for the reader of a failure, what the case is for. It is
	// not recorded in the golden.
	note    string
	method  string
	path    string
	subject string
	// body is marshalled as the request body. A nil body sends no body at all,
	// which is not the same request as one sending "null".
	body any
}

// routeCases is the ordered table. Ordered, not a map, so a failure names the
// same file every run.
func routeCases() []routeCase {
	return []routeCase{
		{
			name:    "create_authenticated",
			note:    "creation behind mere authentication: any valid token mints a profile row",
			method:  http.MethodPost,
			path:    "/api/v1/users",
			subject: subjectA,
			body:    domain.CreateUserRequest{Name: "Carol Example", Email: "carol@example.test"},
		},
		{
			name:    "get_owner",
			note:    "the control: A reads the row this file's convention says is A's",
			method:  http.MethodGet,
			path:    "/api/v1/users/1",
			subject: subjectA,
		},
		{
			name:    "get_cross_user",
			note:    "A reads B's row. Succeeds today. This is E16.",
			method:  http.MethodGet,
			path:    "/api/v1/users/2",
			subject: subjectA,
		},
		{
			name:    "put_cross_user",
			note:    "A overwrites B's name and email. Succeeds today.",
			method:  http.MethodPut,
			path:    "/api/v1/users/2",
			subject: subjectA,
			body:    domain.UpdateUserRequest{Name: "Overwritten By A", Email: "attacker@example.test"},
		},
		{
			name:    "delete_cross_user",
			note:    "A deletes B's row. Succeeds today.",
			method:  http.MethodDelete,
			path:    "/api/v1/users/2",
			subject: subjectA,
		},
		{
			name:    "get_not_found",
			note:    "the comparison baseline: a profile id that genuinely does not exist",
			method:  http.MethodGet,
			path:    "/api/v1/users/999",
			subject: subjectA,
		},
		{
			name: "put_not_found",
			note: "the comparison baseline for PUT: the same write by the same caller, " +
				"aimed at an id that genuinely does not exist. Caller, method and body " +
				"match put_cross_user exactly, so once the ownership check lands, path " +
				"is the ONLY field these two goldens may differ in.",
			method:  http.MethodPut,
			path:    "/api/v1/users/999",
			subject: subjectA,
			body:    domain.UpdateUserRequest{Name: "Overwritten By A", Email: "attacker@example.test"},
		},
		{
			name: "delete_not_found",
			note: "the comparison baseline for DELETE. 204-versus-404 enumerates the id " +
				"space exactly as well as 200-versus-404 does, and this verb had no " +
				"baseline at all until E16-P2.",
			method:  http.MethodDelete,
			path:    "/api/v1/users/999",
			subject: subjectA,
		},
	}
}

// TestCurrentUserRouteShapes drives every case through the assembled route tree
// and records each exchange.
func TestCurrentUserRouteShapes(t *testing.T) {
	for _, tc := range routeCases() {
		t.Run(tc.name, func(t *testing.T) {
			// A fresh store per case: put_cross_user and delete_cross_user
			// mutate it, and a case that inherited the previous case's damage
			// would record a shape no client could ever reproduce.
			repo := newMemoryRepo()
			router := userRouter(repo, tc.subject)

			rec, requestBody := doCase(t, router, tc)

			got := routeGolden{
				Method:        tc.method,
				Path:          tc.path,
				CallerSubject: tc.subject,
				RequestBody:   requestBody,
				Status:        rec.Code,
				ContentType:   rec.Header().Get("Content-Type"),
				Body:          rec.Body.String(),
				StoreAfter:    repo.snapshot(),
			}
			compareGolden(t, filepath.Join(goldenDir, tc.name+".json"), got)
		})
	}
}

// TestIDORIsPresentToday states in code what the goldens record, for the two
// facts that a re-recorded golden would otherwise absorb silently.
//
// It is the companion to the camera above and it will fail when E16 is fixed.
// That is intended, and it is the point of writing it: the engineer landing the
// ownership check has to come here and invert these assertions deliberately,
// rather than run `-update`, see green, and ship. When that happens the second
// subtest becomes the equality the fix actually owes — cross-user and
// not-found, byte-identical — and the first becomes a require.NotEqual.
func TestIDORIsPresentToday(t *testing.T) {
	t.Run("cross_user_read_returns_the_other_users_row", func(t *testing.T) {
		// What A gets when A asks for B's row...
		repoA := newMemoryRepo()
		crossUser := testsupport.Do(t, userRouter(repoA, subjectA),
			testsupport.JSONRequest(t, http.MethodGet, "/api/v1/users/2", nil))

		// ...and what B gets when B asks for B's own row.
		repoB := newMemoryRepo()
		owner := testsupport.Do(t, userRouter(repoB, subjectB),
			testsupport.JSONRequest(t, http.MethodGet, "/api/v1/users/2", nil))

		require.Equal(t, http.StatusOK, crossUser.Code,
			"recorded fact, not an endorsement: the cross-user read succeeds today")
		require.Equal(t, owner.Body.String(), crossUser.Body.String(),
			"A receives byte-for-byte what B receives; the response carries no trace "+
				"of who asked. This is the IDOR (E16).")
	})

	t.Run("cross_user_read_differs_from_not_found", func(t *testing.T) {
		// The enumeration oracle, stated as a value rather than left implicit
		// in two golden files. An attacker walking ids apart tells "exists but
		// is not yours" from "does not exist" by status alone.
		repoExisting := newMemoryRepo()
		existing := testsupport.Do(t, userRouter(repoExisting, subjectA),
			testsupport.JSONRequest(t, http.MethodGet, "/api/v1/users/2", nil))

		repoMissing := newMemoryRepo()
		missing := testsupport.Do(t, userRouter(repoMissing, subjectA),
			testsupport.JSONRequest(t, http.MethodGet, "/api/v1/users/999", nil))

		require.Equal(t, http.StatusOK, existing.Code)
		require.Equal(t, http.StatusNotFound, missing.Code)
		// E16: invert to require.Equal when the ownership check lands
		require.NotEqual(t, missing.Body.String(), existing.Body.String(),
			"the two answers differ, so the id space is enumerable")
	})

	// The same oracle on the other two vulnerable verbs. E23: the obligation
	// applies to all three, and until E16-P2 only GET had a recorded baseline
	// to check the fix against — so a fix could have closed the read oracle,
	// left PUT and DELETE announcing existence, and passed.
	t.Run("cross_user_write_differs_from_not_found", func(t *testing.T) {
		body := domain.UpdateUserRequest{Name: "Overwritten By A", Email: "attacker@example.test"}

		repoExisting := newMemoryRepo()
		existing := testsupport.Do(t, userRouter(repoExisting, subjectA),
			testsupport.JSONRequest(t, http.MethodPut, "/api/v1/users/2", body))

		repoMissing := newMemoryRepo()
		missing := testsupport.Do(t, userRouter(repoMissing, subjectA),
			testsupport.JSONRequest(t, http.MethodPut, "/api/v1/users/999", body))

		// E16: invert to require.Equal when the ownership check lands
		require.NotEqual(t, missing.Code, existing.Code,
			"200 against B's row versus 404 against a free id: the write path announces "+
				"which ids exist")
		// E16: invert to require.Equal when the ownership check lands
		require.NotEqual(t, missing.Body.String(), existing.Body.String())

		// The response is only half of it. A refused write that still wrote
		// would be byte-identical to a refusal and would still have destroyed
		// the row — see StoreAfter's note above.
		// E16: invert to require.Equal when the ownership check lands
		require.NotEqual(t, repoMissing.snapshot(), repoExisting.snapshot(),
			"B's row was overwritten; the not-found probe left the store intact")
	})

	t.Run("cross_user_delete_differs_from_not_found", func(t *testing.T) {
		repoExisting := newMemoryRepo()
		existing := testsupport.Do(t, userRouter(repoExisting, subjectA),
			testsupport.JSONRequest(t, http.MethodDelete, "/api/v1/users/2", nil))

		repoMissing := newMemoryRepo()
		missing := testsupport.Do(t, userRouter(repoMissing, subjectA),
			testsupport.JSONRequest(t, http.MethodDelete, "/api/v1/users/999", nil))

		// E16: invert to require.Equal when the ownership check lands
		require.NotEqual(t, missing.Code, existing.Code,
			"204 against B's row versus 404 against a free id: deleting enumerates just "+
				"as well as reading")
		// E16: invert to require.Equal when the ownership check lands
		require.NotEqual(t, repoMissing.snapshot(), repoExisting.snapshot(),
			"B's row is gone; the not-found probe left the store intact")
	})
}

// doCase issues one case's request and returns the recorder plus the exact
// bytes that were sent as the body (empty when the case sends none).
func doCase(t *testing.T, router http.Handler, tc routeCase) (*httptest.ResponseRecorder, string) {
	t.Helper()

	requestBody := ""
	if tc.body != nil {
		encoded, err := json.Marshal(tc.body)
		require.NoError(t, err)
		requestBody = string(encoded)
	}

	req := testsupport.JSONRequest(t, tc.method, tc.path, tc.body)
	return testsupport.Do(t, router, req), requestBody
}

// userRouter mounts UserHandler's real route tree over the real application
// service and an in-memory repository, behind a middleware that authenticates
// every request as subject.
//
// The authentication is faked deliberately. A1 wired the real RS256 verifier
// because token verification was the thing under test; here it is not — every
// case below is a *successfully* authenticated caller, and what is being
// captured is what happens after that point. testsupport.FakeAuth puts an
// authn.Principal in the context, which is the entire contract the handler has
// with authentication, and it keeps the goldens free of per-run key material.
//
// It is worth noticing that the handler never reads that Principal. Not once,
// in any of the four routes. That is not an oversight in this rig — it is the
// finding, and user_test.go's TestRoutes_AuthenticatedRequestsReachTheHandlers
// already says so in prose. This file is the evidence behind that paragraph.
//
// It does not reuse that file's mount() helper, which pairs a fake service with
// a bare router: the goldens need the real application service (so the recorded
// bodies are the ones the use cases actually produce) and the real /api/v1
// prefix (so the recorded paths are the ones a client types).
func userRouter(repo domain.Repository, subject string) http.Handler {
	h := NewUserHandler(application.NewService(repo), testsupport.FakeAuth(subject))

	r := chi.NewRouter()
	r.Route("/api/v1", h.Routes)
	return r
}

// compareGolden writes want to path under -update, and otherwise fails with a
// readable diff of the two encodings.
//
// Same shape as A1's, deliberately: two golden helpers that drift apart is how
// a repository ends up with two golden conventions.
func compareGolden(t *testing.T, path string, got routeGolden) {
	t.Helper()

	encoded, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)
	encoded = append(encoded, '\n')

	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, encoded, 0o644))
		t.Logf("wrote %s", path)
		return
	}

	want, err := os.ReadFile(path) //nolint:gosec // fixed test-data path
	if err != nil {
		t.Fatalf("read golden %s: %v\nrun the test with -update to create it", path, err)
	}

	// Compared as text so that a change in field order or indentation shows up
	// too: these files are read by humans auditing what the routes used to do.
	if string(want) != string(encoded) {
		t.Errorf("response no longer matches %s\n--- want (golden)\n%s\n--- got\n%s\n"+
			"if the change is intended, rerun with -update and commit the diff",
			path, want, encoded)
	}
}

// memoryRepo is domain.Repository over a map.
//
// It mirrors the bun repository's observable contract rather than its SQL:
// a miss is (nil, nil) and not an error — that is what
// modules/user/infrastructure/persistence/repository/user.go returns when it
// sees sql.ErrNoRows, and the application layer is written against it.
//
// Reads hand back copies. The real repository materializes a fresh entity per
// query, and application.UpdateUser depends on that: it mutates the entity it
// read and calls Update. A fake that returned the stored pointer would let the
// mutation land before Update ran, and the PUT case would appear to work for
// the wrong reason.
type memoryRepo struct {
	rows   map[int64]domain.User
	nextID int64
}

// newMemoryRepo returns the fixture: two profile rows, one "belonging" to each
// subject by the convention documented above.
func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		rows: map[int64]domain.User{
			1: {
				ID:        1,
				Name:      "Alice Example",
				Email:     "alice@example.test",
				CreatedAt: fixtureCreatedAt,
				UpdatedAt: fixtureUpdatedAt,
			},
			2: {
				ID:        2,
				Name:      "Bob Example",
				Email:     "bob@example.test",
				CreatedAt: fixtureCreatedAt,
				UpdatedAt: fixtureUpdatedAt,
			},
		},
		nextID: 3,
	}
}

func (m *memoryRepo) Create(_ context.Context, u *domain.User) error {
	u.ID = m.nextID
	m.nextID++
	// Set here rather than left zero because the bun model declares
	// created_at/updated_at NOT NULL: a row reaches the table with values, and
	// a golden showing the zero time would be recording this fake instead of
	// the route.
	u.CreatedAt = fixtureCreatedAt
	u.UpdatedAt = fixtureUpdatedAt
	m.rows[u.ID] = *u
	return nil
}

func (m *memoryRepo) GetByID(_ context.Context, id int64) (*domain.User, error) {
	row, ok := m.rows[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (m *memoryRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	// Case-insensitive, matching the real repository's lower(email) predicate.
	ids := make([]int64, 0, len(m.rows))
	for id := range m.rows {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for _, id := range ids {
		row := m.rows[id]
		if strings.EqualFold(row.Email, email) {
			return &row, nil
		}
	}
	return nil, nil
}

func (m *memoryRepo) Update(_ context.Context, u *domain.User) error {
	// UpdatedAt is not touched. Neither does the real repository: it writes the
	// entity's own timestamp back and re-reads nothing, so a PUT leaves
	// updated_at exactly as the preceding SELECT found it. Recorded, not fixed.
	m.rows[u.ID] = *u
	return nil
}

func (m *memoryRepo) Delete(_ context.Context, id int64) error {
	delete(m.rows, id)
	return nil
}

// snapshot renders the store, ordered by id, for the golden's store_after
// member.
func (m *memoryRepo) snapshot() []string {
	ids := make([]int64, 0, len(m.rows))
	for id := range m.rows {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	out := make([]string, 0, len(ids))
	for _, id := range ids {
		row := m.rows[id]
		out = append(out, fmt.Sprintf("id=%d name=%q email=%q", row.ID, row.Name, row.Email))
	}
	return out
}
