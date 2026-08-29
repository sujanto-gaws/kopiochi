package transport

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	domain "github.com/sujanto-gaws/kopiochi/modules/user/domain"
)

// E16-P and E16-P2 recorded what this module's routes DID, before the fix, into
// testdata/golden: a cross-user GET returning B's row, a cross-user PUT
// overwriting it, a cross-user DELETE destroying it, an unrestricted POST, and
// the not-found probes that quantified the enumeration oracle.
//
// The plan was to invert those recordings once an ownership check landed. It
// did not land, because E24 answered the prior question — after E20 the profile
// has no field a caller supplies — and the routes were removed instead. There
// is no ownership check to invert: an endpoint that takes no id cannot be asked
// for somebody else's row.
//
// So the goldens are not deleted and they are not inert. They are the exact set
// of requests that used to succeed, and this file replays every one of them
// against the current route table. That makes the record of the defect into the
// regression test for it: if any of these ever routes again, the shape that
// carried the IDOR is back, and this fails by name.
//
// The files themselves are left byte-for-byte as recorded. They are evidence,
// and evidence that gets rewritten to match the present is not evidence.

// recordedRequest is the part of a golden this file needs.
type recordedRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
}

func loadRecorded(t *testing.T) map[string]recordedRequest {
	t.Helper()

	dir := filepath.Join("testdata", "golden")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	out := map[string]recordedRequest{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // fixed test-data path
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var rec recordedRequest
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatalf("%s is not a golden: %v", e.Name(), err)
		}
		out[strings.TrimSuffix(e.Name(), ".json")] = rec
	}

	if len(out) == 0 {
		t.Fatal("no goldens found; this test proves nothing without them")
	}
	return out
}

// TestEveryRecordedIDORRequestIsNowUnroutable replays E16-P's and E16-P2's
// recordings against the live route table.
//
// The assertion is deliberately not "returns 404". A 404 produced by a handler
// that looked the row up and decided not to show it would still be a handler
// that CAN look up somebody else's row. The assertion is that the application
// layer is never entered at all: the request does not resolve to a route, so
// there is nothing to decide.
func TestEveryRecordedIDORRequestIsNowUnroutable(t *testing.T) {
	for name, rec := range loadRecorded(t) {
		t.Run(name, func(t *testing.T) {
			svc := &fakeService{
				getResp:    &domain.UserResponse{ID: testSubject},
				ensureResp: &domain.UserResponse{ID: testSubject},
			}

			r := chi.NewRouter()
			h := NewUserHandler(svc, testsupport.FakeAuth(testSubject.String()))
			r.Route("/api/v1", h.Routes)

			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, httptest.NewRequest(rec.Method, rec.Path, nil))

			if resp.Code >= 200 && resp.Code < 300 {
				t.Errorf("%s %s answered %d, and it answered %d when this golden was recorded. "+
					"The route is back, and with it the shape that carried E16.",
					rec.Method, rec.Path, resp.Code, rec.Status)
			}
			if svc.calls != 0 {
				t.Errorf("%s %s reached the application layer %d times. It must not resolve to "+
					"a route at all — a handler that can be entered with somebody else's id is "+
					"a handler that can leak one.", rec.Method, rec.Path, svc.calls)
			}
		})
	}
}

// TestTheRecordedRequestsIncludedTheCrossUserOnes guards the guard.
//
// If the goldens were ever emptied, thinned, or replaced with recordings of the
// new routes, the test above would pass over whatever was left and report
// nothing. These are the four that mattered, named, so that deleting the
// evidence breaks the build rather than quietly weakening it.
func TestTheRecordedRequestsIncludedTheCrossUserOnes(t *testing.T) {
	recorded := loadRecorded(t)

	for _, name := range []string{
		"get_cross_user", "put_cross_user", "delete_cross_user", "create_authenticated",
	} {
		rec, ok := recorded[name]
		if !ok {
			t.Fatalf("golden %q is gone; it is the record of what E16 could do", name)
		}
		if rec.Status < 200 || rec.Status >= 300 {
			t.Errorf("golden %q records status %d; it was recorded because it SUCCEEDED", name, rec.Status)
		}
	}
}

// TestTheReplayedIDsAreNotTheCaller: the recordings address rows 1, 2 and 999,
// none of which is the authenticated subject. If the paths were ever rewritten
// to the caller's own id the replay above would be testing nothing.
func TestTheReplayedIDsAreNotTheCaller(t *testing.T) {
	for name, rec := range loadRecorded(t) {
		if strings.Contains(rec.Path, testSubject.String()) {
			t.Errorf("golden %q addresses the caller's own id; it was recorded as a cross-user "+
				"request and must stay one", name)
		}
	}
}
