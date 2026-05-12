package auth

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type MfaRequiredResponse struct {
	MFARequired bool    `json:"mfa_required"`
	MFAToken    string  `json:"mfa_token"`
	User        UserDTO `json:"user"`
}

type UserDTO struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

type MfaVerifyRequest struct {
	Code       string `json:"code,omitempty"`
	BackupCode string `json:"backup_code,omitempty"`
}

type MfaSetupResponse struct {
	Secret    string `json:"secret"`
	QRCodeURL string `json:"qrCodeUrl"`
}

type MfaVerifySetupRequest struct {
	Code string `json:"code"`
}

type MfaVerifySetupResponse struct {
	BackupCodes []string `json:"backup_codes"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token,omitempty"`
}