package application

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
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	// Roles and Permissions are ADVISORY and are enforced by nothing.
	//
	// No route, middleware or use case in this service consults them — see E26.
	// They are returned because a client may reasonably want to show a user what
	// their account claims to be, and removing them would break that for no
	// security gain while nothing is enforced either way.
	//
	// Do NOT use them as an access-control decision, here or downstream. Since
	// E26 they are deliberately absent from the access token for that reason: a
	// signed claim travels and gets trusted, a field in a response a client
	// asked about itself does not.
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
