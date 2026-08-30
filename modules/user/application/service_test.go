package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/sujanto-gaws/kopiochi/modules/user/domain"
)

// fakeRepo is an in-memory profile store with the repository's idempotency
// contract: EnsureExists on an id that already has a profile is a no-op, not an
// error.
type fakeRepo struct {
	mu sync.Mutex

	rows      map[uuid.UUID]*domain.User
	ensureErr error
	getErr    error

	ensureCalls int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[uuid.UUID]*domain.User{}}
}

func (r *fakeRepo) EnsureExists(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureCalls++
	if r.ensureErr != nil {
		return r.ensureErr
	}
	if _, ok := r.rows[id]; ok {
		return nil
	}
	now := time.Now()
	r.rows[id] = &domain.User{ID: id, CreatedAt: now, UpdatedAt: now}
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.getErr != nil {
		return nil, r.getErr
	}
	u, ok := r.rows[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

// TestEnsureOwnProfileCreatesTheCallersProfile: the id written is the caller's,
// and it is the only id in scope. Before E16 this use case took an id from the
// request body, which is how a caller could mint a row for anybody.
func TestEnsureOwnProfileCreatesTheCallersProfile(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	caller := uuid.New()

	got, err := svc.EnsureOwnProfile(context.Background(), caller)
	if err != nil {
		t.Fatalf("EnsureOwnProfile: %v", err)
	}
	if got.ID != caller {
		t.Errorf("profile id = %v, want the caller %v", got.ID, caller)
	}
	if _, ok := repo.rows[caller]; !ok {
		t.Error("no profile was stored for the caller")
	}
}

// TestEnsureOwnProfileIsIdempotent: a caller asking twice has not made a
// mistake, and the second request is indistinguishable from a retry of the
// first. Answering one of them with an error would hand a client a failure it
// cannot act on.
func TestEnsureOwnProfileIsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	caller := uuid.New()

	first, err := svc.EnsureOwnProfile(context.Background(), caller)
	if err != nil {
		t.Fatalf("first EnsureOwnProfile: %v", err)
	}
	second, err := svc.EnsureOwnProfile(context.Background(), caller)
	if err != nil {
		t.Fatalf("second EnsureOwnProfile: %v", err)
	}

	if first.ID != second.ID || !first.CreatedAt.Equal(second.CreatedAt) {
		t.Errorf("the second call returned a different profile: %+v vs %+v", first, second)
	}
	if len(repo.rows) != 1 {
		t.Errorf("store holds %d profiles, want 1", len(repo.rows))
	}
}

// TestGetOwnProfileReportsNotFound: a caller who has never created a profile
// gets a 404's worth of information, not a fabricated empty one.
func TestGetOwnProfileReportsNotFound(t *testing.T) {
	svc := NewService(newFakeRepo())

	_, err := svc.GetOwnProfile(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

// TestGetOwnProfileReturnsOnlyTheCallersRow is the IDOR, asserted at the layer
// where it used to live. There is no argument that could ask for B's profile,
// so the check is that A's caller id reaches the store unmodified and B's row
// is never consulted.
func TestGetOwnProfileReturnsOnlyTheCallersRow(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	a, b := uuid.New(), uuid.New()

	if _, err := svc.EnsureOwnProfile(context.Background(), a); err != nil {
		t.Fatalf("seed A: %v", err)
	}
	if _, err := svc.EnsureOwnProfile(context.Background(), b); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	got, err := svc.GetOwnProfile(context.Background(), a)
	if err != nil {
		t.Fatalf("GetOwnProfile: %v", err)
	}
	if got.ID != a {
		t.Errorf("A received profile %v, want its own %v", got.ID, a)
	}
	if got.ID == b {
		t.Fatal("A received B's profile")
	}
}

// TestEnsureOwnProfilePropagatesStoreFailures: a create that did not happen
// must not be reported as a profile.
func TestEnsureOwnProfilePropagatesStoreFailures(t *testing.T) {
	repo := newFakeRepo()
	repo.ensureErr = errors.New("connection closed")
	svc := NewService(repo)

	if _, err := svc.EnsureOwnProfile(context.Background(), uuid.New()); err == nil {
		t.Fatal("EnsureOwnProfile reported success after the store failed")
	}
}

// TestGetOwnProfilePropagatesStoreFailures: an unreachable database is not the
// same answer as "you have no profile", and collapsing the two would tell a
// caller their account is gone during an outage.
func TestGetOwnProfilePropagatesStoreFailures(t *testing.T) {
	repo := newFakeRepo()
	repo.getErr = errors.New("connection closed")
	svc := NewService(repo)

	_, err := svc.GetOwnProfile(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("GetOwnProfile reported success after the store failed")
	}
	if errors.Is(err, domain.ErrUserNotFound) {
		t.Error("a store failure was reported as ErrUserNotFound")
	}
}
