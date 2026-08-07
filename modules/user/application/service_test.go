package application

import (
	"context"
	"errors"
	"testing"

	domain "github.com/sujanto-gaws/kopiochi/modules/user/domain"
)

var errBoom = errors.New("database is down")

// fakeRepo is an in-memory domain.Repository.
//
// GetByID and GetByEmail return (nil, nil) for a missing row rather than an
// error, because that is what the bun repository does — and the service has
// explicit nil checks for it. A fake that returned an error instead would make
// those checks look tested when they never ran.
type fakeRepo struct {
	byID map[int64]*domain.User

	createErr error
	getErr    error
	updateErr error
	deleteErr error

	created []*domain.User
	updated []*domain.User
	deleted []int64
	nextID  int64
}

func newFakeRepo(users ...*domain.User) *fakeRepo {
	r := &fakeRepo{byID: map[int64]*domain.User{}, nextID: 1}
	for _, u := range users {
		r.byID[u.ID] = u
		if u.ID >= r.nextID {
			r.nextID = u.ID + 1
		}
	}
	return r
}

func (r *fakeRepo) Create(_ context.Context, u *domain.User) error {
	if r.createErr != nil {
		return r.createErr
	}
	u.ID = r.nextID
	r.nextID++
	r.byID[u.ID] = u
	r.created = append(r.created, u)
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id int64) (*domain.User, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.byID[id], nil // nil, nil when absent — as the real repository does
}

func (r *fakeRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	for _, u := range r.byID {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, nil
}

func (r *fakeRepo) Update(_ context.Context, u *domain.User) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.byID[u.ID] = u
	r.updated = append(r.updated, u)
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, id int64) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	delete(r.byID, id)
	r.deleted = append(r.deleted, id)
	return nil
}

func existingUser() *domain.User {
	return &domain.User{ID: 1, Name: "Alice", Email: "alice@example.com"}
}

func TestCreateUser_Valid(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	svc := NewService(repo)

	got, err := svc.CreateUser(context.Background(), &domain.CreateUserRequest{
		Name: "Alice", Email: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if got.Name != "Alice" || got.Email != "alice@example.com" {
		t.Errorf("CreateUser() = %+v", got)
	}
	if got.ID == 0 {
		t.Error("CreateUser() returned an id of 0; the caller cannot address the row")
	}
}

// TestCreateUser_InvalidNeverReachesTheRepository: validation must run before
// persistence, or invalid rows land in the table and the check becomes
// decorative.
func TestCreateUser_InvalidNeverReachesTheRepository(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  *domain.CreateUserRequest
		want error
	}{
		{"empty name", &domain.CreateUserRequest{Name: "", Email: "a@example.com"}, domain.ErrInvalidName},
		{"bad email", &domain.CreateUserRequest{Name: "Alice", Email: "not-an-email"}, domain.ErrInvalidEmail},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := newFakeRepo()
			svc := NewService(repo)

			_, err := svc.CreateUser(context.Background(), tc.req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("CreateUser() error = %v, want %v", err, tc.want)
			}
			if len(repo.created) != 0 {
				t.Error("an invalid user was written to the repository")
			}
		})
	}
}

func TestCreateUser_RepositoryErrorIsPropagated(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.createErr = errBoom
	svc := NewService(repo)

	got, err := svc.CreateUser(context.Background(), &domain.CreateUserRequest{
		Name: "Alice", Email: "alice@example.com",
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("CreateUser() error = %v, want errBoom", err)
	}
	if got != nil {
		t.Errorf("CreateUser() = %+v, want nil on failure", got)
	}
}

func TestGetUserByID(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo(existingUser())
	svc := NewService(repo)

	got, err := svc.GetUserByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if got.ID != 1 {
		t.Errorf("GetUserByID() = %+v, want id 1", got)
	}
}

// TestGetUserByID_MissingRowIsNotFound covers the (nil, nil) contract: without
// the service's nil check this would return a nil *UserResponse and a nil
// error, and the handler would dereference it.
func TestGetUserByID_MissingRowIsNotFound(t *testing.T) {
	t.Parallel()

	svc := NewService(newFakeRepo())

	got, err := svc.GetUserByID(context.Background(), 999)
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("GetUserByID() error = %v, want ErrUserNotFound", err)
	}
	if got != nil {
		t.Errorf("GetUserByID() = %+v, want nil", got)
	}
}

// TestGetUserByID_NonPositiveIDShortCircuits: ids are positive, so a zero or
// negative one is a malformed request and must not become a query.
func TestGetUserByID_NonPositiveIDShortCircuits(t *testing.T) {
	t.Parallel()

	for _, id := range []int64{0, -1} {
		repo := newFakeRepo(existingUser())
		repo.getErr = errBoom // would surface if the repository were consulted
		svc := NewService(repo)

		_, err := svc.GetUserByID(context.Background(), id)
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("GetUserByID(%d) error = %v, want ErrUserNotFound without touching the repository", id, err)
		}
	}
}

func TestGetUserByEmail_MissingRowIsNotFound(t *testing.T) {
	t.Parallel()

	svc := NewService(newFakeRepo(existingUser()))

	_, err := svc.GetUserByEmail(context.Background(), "nobody@example.com")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("GetUserByEmail() error = %v, want ErrUserNotFound", err)
	}
}

func TestUpdateUser_Valid(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo(existingUser())
	svc := NewService(repo)

	got, err := svc.UpdateUser(context.Background(), 1, &domain.UpdateUserRequest{
		Name: "Alice Smith", Email: "alice.smith@example.com",
	})
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}
	if got.Name != "Alice Smith" || got.Email != "alice.smith@example.com" {
		t.Errorf("UpdateUser() = %+v", got)
	}
	if got.ID != 1 {
		t.Errorf("UpdateUser() changed the id to %d; the id must survive an update", got.ID)
	}
}

func TestUpdateUser_MissingRowIsNotFound(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	svc := NewService(repo)

	_, err := svc.UpdateUser(context.Background(), 999, &domain.UpdateUserRequest{
		Name: "Alice", Email: "alice@example.com",
	})
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("UpdateUser() error = %v, want ErrUserNotFound", err)
	}
	if len(repo.updated) != 0 {
		t.Error("a non-existent user was written")
	}
}

// TestUpdateUser_InvalidIsNotPersisted: the update path validates *after*
// applying the new values to the loaded entity, so this is the check that
// stops an invalid email overwriting a valid one.
func TestUpdateUser_InvalidIsNotPersisted(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo(existingUser())
	svc := NewService(repo)

	_, err := svc.UpdateUser(context.Background(), 1, &domain.UpdateUserRequest{
		Name: "Alice", Email: "not-an-email",
	})
	if !errors.Is(err, domain.ErrInvalidEmail) {
		t.Fatalf("UpdateUser() error = %v, want ErrInvalidEmail", err)
	}
	if len(repo.updated) != 0 {
		t.Error("an invalid update was written to the repository")
	}
}

func TestDeleteUser(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo(existingUser())
	svc := NewService(repo)

	if err := svc.DeleteUser(context.Background(), 1); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != 1 {
		t.Errorf("deleted = %v, want [1]", repo.deleted)
	}
}

// TestDeleteUser_MissingRowIsNotFound: deleting a row that is not there must
// report not-found rather than succeeding silently, or a caller cannot tell a
// successful delete from a wrong id.
func TestDeleteUser_MissingRowIsNotFound(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.DeleteUser(context.Background(), 999); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("DeleteUser() error = %v, want ErrUserNotFound", err)
	}
	if len(repo.deleted) != 0 {
		t.Error("a delete was issued for a row that does not exist")
	}
}
