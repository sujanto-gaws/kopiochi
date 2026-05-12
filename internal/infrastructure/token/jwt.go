package token

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"
	domain "github.com/sujanto-gaws/kopiochi/internal/domain/auth"
	"github.com/golang-jwt/jwt/v5"
)

type JWTService struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	issuer     string
}

func NewJWTService(privateKeyPath, publicKeyPath, issuer string) (*JWTService, error) {
	priv, err := loadRSAPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}
	pub, err := loadRSAPublicKey(publicKeyPath)
	if err != nil {
		return nil, err
	}
	return &JWTService{privateKey: priv, publicKey: pub, issuer: issuer}, nil
}

func (s *JWTService) IssueAccessToken(user domain.User, ttl time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"sub":         user.ID.String(),
		"email":       user.Email,
		"name":        user.Name,
		"roles":       user.Roles,
		"permissions": user.Permissions,
		"scope":       "access",
		"iss":         s.issuer,
		"iat":         time.Now().Unix(),
		"exp":         time.Now().Add(ttl).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.privateKey)
}

func (s *JWTService) IssueIDToken(user domain.User, clientID string) (string, error) {
	claims := jwt.MapClaims{
		"sub":         user.ID.String(),
		"email":       user.Email,
		"name":        user.Name,
		"aud":         clientID,
		"iss":         s.issuer,
		"iat":         time.Now().Unix(),
		"exp":         time.Now().Add(15 * time.Minute).Unix(),
		"scope":       "openid profile email",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.privateKey)
}

func (s *JWTService) IssueMFAToken(user domain.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":   user.ID.String(),
		"scope": "mfa",
		"iss":   s.issuer,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.privateKey)
}

func (s *JWTService) Validate(tokenStr string) (*domain.Claims, error) {
	parsed, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.publicKey, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		return nil, fmt.Errorf("invalid token")
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