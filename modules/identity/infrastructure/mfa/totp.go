// Package mfa implements the identity domain's TOTP second-factor port.
package mfa

import (
	"github.com/pquerna/otp/totp"
)

type TOTPService struct {
	Issuer string
}

func NewTOTPService(issuer string) *TOTPService {
	return &TOTPService{Issuer: issuer}
}

func (s *TOTPService) GenerateSecret(email string) (secret string, qrURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      s.Issuer,
		AccountName: email,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ValidateCode reports whether code is a valid TOTP for secret.
//
// An EMPTY SECRET IS ALWAYS INVALID, and that guard is the whole reason this
// method is not a one-liner (E10).
//
// totp.Validate(code, "") returns TRUE for the code derived from the empty
// secret. It is not that any code passes — "000000" does not — it is that THE
// code is computable by anyone with the current time and three lines of Go:
//
//	totp.GenerateCode("", time.Now())   -> a code
//	totp.Validate(thatCode, "")         -> true
//
// mfa_secret is a nullable column mapped to a plain string, so "MFA is on and
// the secret is empty" is representable, and every account that has never run
// setup is in exactly that state. Without this guard, the second factor for
// such an account is a public constant.
//
// Fail closed: a second factor that cannot be checked has not been provided.
func (s *TOTPService) ValidateCode(secret, code string) bool {
	if secret == "" {
		return false
	}
	return totp.Validate(code, secret)
}
