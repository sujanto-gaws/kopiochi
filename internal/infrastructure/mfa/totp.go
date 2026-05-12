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

func (s *TOTPService) ValidateCode(secret, code string) bool {
	return totp.Validate(code, secret)
}