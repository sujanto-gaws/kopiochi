package transport

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
)

// walk returns the mounted route table as "METHOD path", sorted.
//
// It walks the real chi tree rather than reading the source of Routes, so a
// route that is registered but shadowed, or registered on the wrong router, is
// visible here.
func walk(t *testing.T, h *Handler) []string {
	t.Helper()

	r := chi.NewRouter()
	r.Route("/api/v1", h.Routes)

	var got []string
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got = append(got, method+" "+route)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Strings(got)
	return got
}

// TestRouteTableIsExactlyTheBlueprint pins the surface in both directions.
//
// The absences matter more than the presences. This module's routes carry an
// id, which modules/user's no longer do, so the table is the place a route that
// names a RECIPIENT — /notifications/user/{id}, an admin list, a send endpoint —
// would first appear. Asserting the exact set means such a route arrives as a
// failure here rather than as a review nobody ran.
func TestRouteTableIsExactlyTheBlueprint(t *testing.T) {
	got := walk(t, NewHandler(&fakeService{}, testsupport.FakeAuth(testSubject.String())))

	want := []string{
		"GET /api/v1/notifications",
		"GET /api/v1/notifications/preferences",
		"POST /api/v1/notifications/read-all",
		"POST /api/v1/notifications/{id}/read",
		"PUT /api/v1/notifications/preferences",
	}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("route table is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestTheOnlyPathParameterNamesARowAndNotARecipient.
//
// One parameter, called id, in one route. A second parameter would be the first
// question worth asking about this module's routes — whose row is it — and the
// answer must stay "the caller's, because the Principal says so". A path that
// carried a user id would be E16 rediscovered with a different table behind it.
func TestTheOnlyPathParameterNamesARowAndNotARecipient(t *testing.T) {
	for _, route := range walk(t, NewHandler(&fakeService{}, testsupport.FakeAuth(testSubject.String()))) {
		params := strings.Count(route, "{")
		if params > 1 {
			t.Errorf("%s has %d path parameters; one of them is not the row", route, params)
		}
		for _, forbidden := range []string{"{user", "{recipient", "{subject", "{account"} {
			if strings.Contains(route, forbidden) {
				t.Errorf("%s names a recipient in the path; the recipient is the Principal", route)
			}
		}
	}
}

// TestEveryRouteIsInsideTheAuthGroup is the fail-open guard at module scope. A
// route registered outside the r.Use(h.authMW) group would serve a mailbox to
// anonymous callers, and it would look exactly like the others in the table
// above.
//
// The middleware here counts instead of authenticating, so "did the chain run"
// is observable per request. It still injects a Principal, because the handlers
// call authn.MustFromContext and a route that reached one without it would
// panic rather than report.
func TestEveryRouteIsInsideTheAuthGroup(t *testing.T) {
	for _, tc := range everyRoute {
		t.Run(tc.name, func(t *testing.T) {
			var ran int
			inner := testsupport.FakeAuth(testSubject.String())
			probe := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					ran++
					inner(next).ServeHTTP(w, r)
				})
			}

			r := chi.NewRouter()
			NewHandler(&fakeService{}, probe).Routes(r)

			rec := doJSON(t, r, tc.method, tc.path, `{"preferences":[]}`)
			if rec.Code >= 500 {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body)
			}
			if ran != 1 {
				t.Errorf("the auth middleware ran %d times for %s %s; this route is outside the group",
					ran, tc.method, tc.path)
			}
		})
	}
}
