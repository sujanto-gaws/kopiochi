package auth

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

// Hand-written fakes rather than a mocking library. The interfaces here have
// three to four methods each, the assertions are about behaviour rather than
// call counts, and a generated mock would obscure exactly the thing these
// tests exist to check: what the service does to persisted state on a failed
// login.

var errNotFound = errors.New("not found")

// fakeUserRepo is an in-memory UserRepository. It is mutex-guarded because
// Login writes through it and the concurrency test drives it from several
// goroutines.
type fakeUserRepo struct {
	mu       sync.Mutex
	byName   map[string]*domain.User
	byID     map[string]*domain.User
	saveErr  error
	saveCall int
}

func newFakeUserRepo(users ...*domain.User) *fakeUserRepo {
	r := &fakeUserRepo{
		byName: map[string]*domain.User{},
		byID:   map[string]*domain.User{},
	}
	for _, u := range users {
		r.byName[u.Username] = u
		r.byID[u.ID.String()] = u
	}
	return r
}

func (r *fakeUserRepo) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.byName[username]
	if !ok {
		return nil, errNotFound
	}
	return u, nil
}

func (r *fakeUserRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.byID[id]
	if !ok {
		return nil, errNotFound
	}
	return u, nil
}

func (r *fakeUserRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.byName {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, errNotFound
}

func (r *fakeUserRepo) Save(_ context.Context, u *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.saveCall++
	if r.saveErr != nil {
		return r.saveErr
	}
	r.byName[u.Username] = u
	r.byID[u.ID.String()] = u
	return nil
}

func (r *fakeUserRepo) saves() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveCall
}

// fakeHasher treats a password as correct when it equals the "hash" with the
// hashPrefix stripped. Real bcrypt is exercised in the hasher package's own
// tests; using it here would add ~60ms per case for no extra coverage.
const hashPrefix = "hashed:"

type fakeHasher struct{}

func (fakeHasher) Hash(plain string) (string, error) { return hashPrefix + plain, nil }
func (fakeHasher) Verify(plain, hashed string) bool  { return hashPrefix+plain == hashed }

// fakeTokenIssuer returns predictable token strings and records what it was
// asked to mint, so a test can assert which class was issued.
type fakeTokenIssuer struct {
	mu sync.Mutex

	accessErr error
	mfaErr    error

	issuedAccess []string
	issuedMFA    []string

	// validate is consulted by Validate; nil means "reject everything".
	validate func(tokenStr string, want domain.Class) (*domain.Claims, error)
}

func (f *fakeTokenIssuer) IssueAccessToken(u domain.User, _ time.Duration) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.accessErr != nil {
		return "", f.accessErr
	}
	f.issuedAccess = append(f.issuedAccess, u.ID.String())
	return "access-token-for-" + u.ID.String(), nil
}

func (f *fakeTokenIssuer) IssueIDToken(u domain.User, _ string) (string, error) {
	return "id-token-for-" + u.ID.String(), nil
}

func (f *fakeTokenIssuer) IssueMFAToken(u domain.User) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.mfaErr != nil {
		return "", f.mfaErr
	}
	f.issuedMFA = append(f.issuedMFA, u.ID.String())
	return "mfa-token-for-" + u.ID.String(), nil
}

func (f *fakeTokenIssuer) Validate(tokenStr string, want domain.Class) (*domain.Claims, error) {
	if f.validate == nil {
		return nil, errors.New("invalid token")
	}
	return f.validate(tokenStr, want)
}

func (f *fakeTokenIssuer) accessTokensIssued() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.issuedAccess)
}

// fakeTokenStore is an in-memory RefreshTokenStore keyed by hash.
type fakeTokenStore struct {
	mu              sync.Mutex
	byHash          map[string]domain.RefreshToken
	storeErr        error
	rotateErr       error
	revokeFamilyErr error
	revoked         []string
	revokedFamilies []string
}

func newFakeTokenStore() *fakeTokenStore {
	return &fakeTokenStore{byHash: map[string]domain.RefreshToken{}}
}

func (s *fakeTokenStore) Store(_ context.Context, tok domain.RefreshToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.storeErr != nil {
		return s.storeErr
	}
	if tok.FamilyID == "" {
		// A fresh login starts its own family, as the real store does.
		tok.FamilyID = uuid.New().String()
	}
	s.byHash[tok.TokenHash] = tok
	return nil
}

func (s *fakeTokenStore) FindValid(_ context.Context, hash string) (*domain.RefreshToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tok, ok := s.byHash[hash]
	if !ok || !tok.Usable(time.Now()) {
		return nil, errNotFound
	}
	return &tok, nil
}

// FindAny mirrors the real store: it returns spent and revoked tokens too, so
// the service can tell a stolen credential from one that never existed.
func (s *fakeTokenStore) FindAny(_ context.Context, hash string) (*domain.RefreshToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tok, ok := s.byHash[hash]
	if !ok {
		return nil, errNotFound
	}
	return &tok, nil
}

// Rotate marks the old token used and stores the new one, refusing if the old
// one was already spent — the same rowsAffected==0 guard the real store gets
// from SQL.
func (s *fakeTokenStore) Rotate(_ context.Context, oldHash string, next domain.RefreshToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rotateErr != nil {
		return s.rotateErr
	}

	old, ok := s.byHash[oldHash]
	if !ok || old.Used || old.Revoked {
		return domain.ErrRefreshTokenAlreadyUsed
	}

	old.Used = true
	old.Revoked = true
	s.byHash[oldHash] = old

	if s.storeErr != nil {
		return s.storeErr
	}
	s.byHash[next.TokenHash] = next
	return nil
}

func (s *fakeTokenStore) RevokeFamily(_ context.Context, familyID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.revokeFamilyErr != nil {
		return 0, s.revokeFamilyErr
	}

	n := 0
	for h, tok := range s.byHash {
		if tok.FamilyID == familyID && !tok.Revoked {
			tok.Revoked = true
			s.byHash[h] = tok
			n++
		}
	}
	s.revokedFamilies = append(s.revokedFamilies, familyID)
	return n, nil
}

func (s *fakeTokenStore) RevokeAllForUser(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.revoked = append(s.revoked, userID)
	for h, tok := range s.byHash {
		if tok.UserID == userID {
			tok.Revoked = true
			s.byHash[h] = tok
		}
	}
	return nil
}

// fakeMFAService accepts one fixed code.
type fakeMFAService struct {
	validCode string
	genErr    error
}

func (f fakeMFAService) GenerateSecret(email string) (string, string, error) {
	if f.genErr != nil {
		return "", "", f.genErr
	}
	return "SECRET-" + email, "otpauth://totp/" + email, nil
}

func (f fakeMFAService) ValidateCode(_, code string) bool {
	return f.validCode != "" && code == f.validCode
}

// fakeMFAStore accepts one fixed backup code, once.
type fakeMFAStore struct {
	mu         sync.Mutex
	backupCode string
	used       bool
	stored     []string
}

func (f *fakeMFAStore) StoreBackupCodes(_ context.Context, _ string, hashes []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.stored = append(f.stored, hashes...)
	return nil
}

func (f *fakeMFAStore) FindAndUseBackupCode(_ context.Context, _ string, plain string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.used || f.backupCode == "" || plain != f.backupCode {
		return false, nil
	}
	f.used = true
	return true, nil
}

// testUser builds a user whose password is testPassword.
const testPassword = "correct horse battery staple"

func testUser(username string) *domain.User {
	return &domain.User{
		ID:           uuid.New(),
		Username:     username,
		Email:        username + "@example.com",
		Name:         "Test " + username,
		Roles:        []string{"user"},
		Permissions:  []string{},
		PasswordHash: hashPrefix + testPassword,
	}
}

// testConfig is a Config with values that make the assertions readable.
func testConfig() Config {
	return Config{
		AccessTokenTTL:    15 * time.Minute,
		RefreshTokenTTL:   168 * time.Hour,
		MaxFailedAttempts: 3,
		LockDuration:      time.Hour,
		ClientID:          "kopiochi-test",
	}
}

// harness bundles a Service with the fakes behind it, so a test can assert on
// what the service did to them.
type harness struct {
	svc      *Service
	users    *fakeUserRepo
	issuer   *fakeTokenIssuer
	tokens   *fakeTokenStore
	mfaSvc   fakeMFAService
	mfaStore *fakeMFAStore
	audit    *recordingAuditor
}

func newHarness(users ...*domain.User) *harness {
	h := &harness{
		users:    newFakeUserRepo(users...),
		issuer:   &fakeTokenIssuer{},
		tokens:   newFakeTokenStore(),
		mfaSvc:   fakeMFAService{validCode: "123456"},
		mfaStore: &fakeMFAStore{backupCode: "backup-code-1"},
		audit:    &recordingAuditor{},
	}
	h.svc = NewService(h.users, fakeHasher{}, h.issuer, h.tokens, testConfig(), h.mfaSvc, h.mfaStore, h.audit)
	return h
}

// recordingAuditor captures emitted events so a test can assert that a
// security-relevant thing was actually recorded.
//
// This is the reason Auditor is an interface rather than a concrete logger:
// "was the reuse detection audited?" is exactly as important as "was the
// family revoked?", and an incident review only ever sees the former.
type recordingAuditor struct {
	mu     sync.Mutex
	events []auditEvent
}

type auditEvent struct {
	Action   string
	ActorID  string
	Subject  string
	Reason   string
	FamilyID string
	Revoked  int
	Err      error
}

func (a *recordingAuditor) record(e auditEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *recordingAuditor) LoginSucceeded(_ context.Context, userID string) {
	a.record(auditEvent{Action: "login.success", ActorID: userID})
}

func (a *recordingAuditor) LoginFailed(_ context.Context, username, reason string) {
	a.record(auditEvent{Action: "login.failure", Subject: username, Reason: reason})
}

func (a *recordingAuditor) AccountLocked(_ context.Context, userID string) {
	a.record(auditEvent{Action: "account.locked", ActorID: userID})
}

func (a *recordingAuditor) LogoutSucceeded(_ context.Context, userID string) {
	a.record(auditEvent{Action: "logout", ActorID: userID})
}

func (a *recordingAuditor) MFAEnrolled(_ context.Context, userID string) {
	a.record(auditEvent{Action: "mfa.enrolled", ActorID: userID})
}

func (a *recordingAuditor) MFAFailed(_ context.Context, userID, reason string) {
	a.record(auditEvent{Action: "mfa.failed", ActorID: userID, Reason: reason})
}

func (a *recordingAuditor) RefreshReuseDetected(_ context.Context, userID, familyID string, revoked int, err error) {
	a.record(auditEvent{
		Action: "refresh.reuse", ActorID: userID, FamilyID: familyID,
		Revoked: revoked, Err: err,
	})
}

// find returns the first event with the given action, and whether one existed.
func (a *recordingAuditor) find(action string) (auditEvent, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, e := range a.events {
		if e.Action == action {
			return e, true
		}
	}
	return auditEvent{}, false
}

// count returns how many events with the given action were emitted.
func (a *recordingAuditor) count(action string) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	n := 0
	for _, e := range a.events {
		if e.Action == action {
			n++
		}
	}
	return n
}
