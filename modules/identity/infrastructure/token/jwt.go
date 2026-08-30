// Package token implements the identity domain's JWT issuing and
// verification ports.
//
// Verification pins the signing algorithm and checks issuer, audience and
// token class, so a token minted for one purpose cannot be replayed as
// another — see docs/architectures/04-security/token-architecture.md.
package token

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"

	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

// defaultLeeway absorbs small clock skew between the process that minted a
// token and the process validating it. It is used whenever the caller does
// not configure a positive leeway explicitly.
const defaultLeeway = 30 * time.Second

// rs256Alg pins every token this service issues and verifies to RS256.
// jwt.WithValidMethods below rejects any other alg — including "none" and
// any HMAC variant — before the keyfunc is even invoked, which is what
// makes the classic "confusion attack" (forging an HS256 token using the
// RSA *public* key, which is public, as the HMAC secret) structurally
// impossible rather than merely unlikely.
var rs256Alg = jwt.SigningMethodRS256.Alg()

type JWTService struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	issuer     string
	audience   string
	leeway     time.Duration

	// signingKid is the thumbprint of publicKey, stamped into the "kid"
	// header of every token this service issues.
	signingKid string
	// verificationKeys is every key this service will accept, by kid. During
	// a rotation it holds both the new signing key and the previous one, so
	// tokens issued before the switch keep verifying until they expire.
	// Without that overlap, rotating a key logs out every active session.
	verificationKeys map[string]*rsa.PublicKey
}

// NewJWTService builds a JWTService that signs with privateKey and verifies
// with publicKey. Every token minted carries iss=issuer and aud=audience,
// and Validate rejects any token whose alg, iss, aud, or exp does not match
// (exp is checked with the given leeway to absorb clock skew).
func NewJWTService(privateKeyPath, publicKeyPath, issuer, audience string, leeway time.Duration) (*JWTService, error) {
	priv, err := loadRSAPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}
	pub, err := loadRSAPublicKey(publicKeyPath)
	if err != nil {
		return nil, err
	}
	if leeway <= 0 {
		leeway = defaultLeeway
	}
	signingKid := Kid(pub)
	return &JWTService{
		privateKey:       priv,
		publicKey:        pub,
		issuer:           issuer,
		audience:         audience,
		leeway:           leeway,
		signingKid:       signingKid,
		verificationKeys: map[string]*rsa.PublicKey{signingKid: pub},
	}, nil
}

// IssueAccessToken mints an access token for user.
//
// It deliberately does NOT carry roles or permissions. It did until E26, and
// nothing anywhere consulted them: a grep for RequireRole, RequirePermission,
// Authorize, HasRole, HasPermission or CanAccess found nothing outside comments,
// and nothing read authn.Principal.Scopes either.
//
// A signed claim that nothing enforces is worse than an absent one. It travels,
// and a downstream service can trust it without ever asking this one — so the
// claim becomes an authorization decision made by whoever reads it, on data this
// service never checks. An absent feature is obvious; a present-looking one gets
// trusted.
//
// The columns and the login response keep them, documented as advisory. The rule
// this follows: a claim is minted when something enforces it, and not before.
// Minting first is how a decorative claim happens. When an authorization
// primitive lands (E26), these come back alongside the code that checks them.
func (s *JWTService) IssueAccessToken(user domain.User, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   user.ID.String(),
		"email": user.Email,
		"name":  user.Name,
		"scope": "access",
		"cls":   string(domain.ClassAccess),
		"iss":   s.issuer,
		"aud":   s.audience,
		"iat":   now.Unix(),
		"exp":   now.Add(ttl).Unix(),
	}
	return s.sign(claims)
}

func (s *JWTService) IssueIDToken(user domain.User, clientID string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   user.ID.String(),
		"email": user.Email,
		"name":  user.Name,
		"aud":   clientID,
		"iss":   s.issuer,
		"cls":   string(domain.ClassID),
		"iat":   now.Unix(),
		"exp":   now.Add(15 * time.Minute).Unix(),
		"scope": "openid profile email",
	}
	return s.sign(claims)
}

func (s *JWTService) IssueMFAToken(user domain.User) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   user.ID.String(),
		"scope": "mfa",
		"cls":   string(domain.ClassMFA),
		"iss":   s.issuer,
		"aud":   s.audience,
		"iat":   now.Unix(),
		"exp":   now.Add(5 * time.Minute).Unix(),
	}
	return s.sign(claims)
}

// sign issues a token, stamping the signing key's thumbprint into the "kid"
// header.
//
// Without kid a verifier holding two keys must try each in turn, which works
// but makes a signature failure ambiguous: it cannot tell "wrong key" from
// "forged token". With it, key selection is exact and a mismatch is
// unambiguous.
func (s *JWTService) sign(claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if s.signingKid != "" {
		token.Header["kid"] = s.signingKid
	}
	return token.SignedString(s.privateKey)
}

// Validate parses tokenStr, verifies its signature under the RS256 public
// key, and requires:
//   - alg == RS256, exactly (jwt.WithValidMethods) — no other algorithm,
//     including "none" or any HMAC variant, is ever accepted;
//   - iss matches s.issuer and aud matches s.audience;
//   - exp is present (jwt.WithExpirationRequired) and not expired, allowing
//     s.leeway for clock skew;
//   - the token's "cls" claim equals want — a token minted for a different
//     class (e.g. an MFA token presented where an access token is expected)
//     is rejected with ErrWrongTokenClass, structurally, not by convention.
func (s *JWTService) Validate(tokenStr string, want domain.Class) (*domain.Claims, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		// Defense in depth: jwt.WithValidMethods below already rejects any
		// alg other than RS256 before this keyfunc runs, but keep the
		// check here too so the keyfunc documents (and enforces) the same
		// invariant if it is ever reused on its own.
		if token.Method != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.keyFor(token)
	},
		jwt.WithValidMethods([]string{rs256Alg}),
		jwt.WithIssuer(s.issuer),
		jwt.WithAudience(s.audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(s.leeway),
	)
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	if !parsed.Valid {
		return nil, errors.New("invalid token")
	}

	cls := domain.Class(getString(claims, "cls"))
	if cls != want {
		return nil, fmt.Errorf("%w: got %q, want %q", domain.ErrWrongTokenClass, cls, want)
	}

	// Roles and Permissions are still READ, and that is not an oversight. Tokens
	// minted before E26 are still valid until they expire, and a validator that
	// dropped the fields would report a different Claims for the same token
	// depending on when it was issued. Reading a claim commits to nothing;
	// minting one does. They will be empty for every token issued from here on.
	c := &domain.Claims{
		Subject:     getString(claims, "sub"),
		Email:       getString(claims, "email"),
		Name:        getString(claims, "name"),
		Roles:       getStringSlice(claims, "roles"),
		Permissions: getStringSlice(claims, "permissions"),
		Scope:       getString(claims, "scope"),
		Class:       cls,
		IssuedAt:    getInt64(claims, "iat"),
		ExpiresAt:   getInt64(claims, "exp"),
	}
	return c, nil
}

func loadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("pem decode failed")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func loadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("pem decode failed")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not RSA public key")
	}
	return rsaPub, nil
}

// helpers for map claims
func getString(c jwt.MapClaims, key string) string {
	v, _ := c[key].(string)
	return v
}
func getStringSlice(c jwt.MapClaims, key string) []string {
	raw, ok := c[key].([]interface{})
	if !ok {
		return nil
	}
	res := make([]string, len(raw))
	for i, r := range raw {
		res[i], _ = r.(string)
	}
	return res
}
func getInt64(c jwt.MapClaims, key string) int64 {
	f, ok := c[key].(float64)
	if ok {
		return int64(f)
	}
	return 0
}

// keyFor selects the verification key for a token.
//
// A token carrying a kid we do not know is rejected outright rather than
// falling back to trying every key. Falling back would quietly undo a
// rotation: after the old key is retired, tokens signed with it would keep
// verifying against whatever else happened to be loaded, if it matched — and
// more practically, it turns key selection into an oracle an attacker can
// probe.
//
// A token with *no* kid is accepted against the signing key. That path exists
// only for tokens minted before kid was introduced, and it disappears
// naturally once they expire.
func (s *JWTService) keyFor(token *jwt.Token) (any, error) {
	raw, ok := token.Header["kid"]
	if !ok {
		return s.publicKey, nil
	}

	kid, ok := raw.(string)
	if !ok {
		return nil, errors.New("token: kid header is not a string")
	}

	key, ok := s.verificationKeys[kid]
	if !ok {
		return nil, fmt.Errorf("token: unknown key id %q", kid)
	}
	return key, nil
}

// SigningKid is the thumbprint of the key this service signs with. Exposed so
// an operator can confirm which key is live without decoding a token.
func (s *JWTService) SigningKid() string { return s.signingKid }

// WithPreviousKey adds a retired public key to the verification set.
//
// This is what makes key rotation survivable. Swapping the signing key without
// keeping the old one for verification invalidates every token already issued:
// every active session is logged out at once, and every client refreshing at
// that moment fails. Keeping the previous key until the longest-lived token it
// signed has expired makes the switch invisible.
//
// The retired key can only verify; the service signs with one key and one key
// only.
func (s *JWTService) WithPreviousKey(path string) error {
	pub, err := loadRSAPublicKey(path)
	if err != nil {
		return fmt.Errorf("load previous public key: %w", err)
	}

	kid := Kid(pub)
	if kid == s.signingKid {
		// Almost certainly a copy-paste of the active key path. Accepting it
		// silently would make the config look like a rotation was configured
		// when nothing was.
		return errors.New("previous key is the same as the signing key")
	}

	s.verificationKeys[kid] = pub
	return nil
}
