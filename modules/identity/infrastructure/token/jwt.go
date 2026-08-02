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
	return &JWTService{
		privateKey: priv,
		publicKey:  pub,
		issuer:     issuer,
		audience:   audience,
		leeway:     leeway,
	}, nil
}

func (s *JWTService) IssueAccessToken(user domain.User, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":         user.ID.String(),
		"email":       user.Email,
		"name":        user.Name,
		"roles":       user.Roles,
		"permissions": user.Permissions,
		"scope":       "access",
		"iss":         s.issuer,
		"aud":         s.audience,
		"iat":         now.Unix(),
		"exp":         now.Add(ttl).Unix(),
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
		"iss":   s.issuer,
		"aud":   s.audience,
		"iat":   now.Unix(),
		"exp":   now.Add(5 * time.Minute).Unix(),
	}
	return s.sign(claims)
}

func (s *JWTService) sign(claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.privateKey)
}

// Validate parses tokenStr, verifies its signature under the RS256 public
// key, and requires:
//   - alg == RS256, exactly (jwt.WithValidMethods) — no other algorithm,
//     including "none" or any HMAC variant, is ever accepted;
//   - iss matches s.issuer and aud matches s.audience;
//   - exp is present (jwt.WithExpirationRequired) and not expired, allowing
//     s.leeway for clock skew.
func (s *JWTService) Validate(tokenStr string) (*domain.Claims, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		// Defense in depth: jwt.WithValidMethods below already rejects any
		// alg other than RS256 before this keyfunc runs, but keep the
		// check here too so the keyfunc documents (and enforces) the same
		// invariant if it is ever reused on its own.
		if token.Method != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.publicKey, nil
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
	// Extract into Claims struct
	c := &domain.Claims{
		Subject:     getString(claims, "sub"),
		Email:       getString(claims, "email"),
		Name:        getString(claims, "name"),
		Roles:       getStringSlice(claims, "roles"),
		Permissions: getStringSlice(claims, "permissions"),
		Scope:       getString(claims, "scope"),
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
