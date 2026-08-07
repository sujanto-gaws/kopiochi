package domain

type WebAuthnCredential struct {
	ID              []byte
	UserID          string
	PublicKey       []byte
	AttestationType string
	AAGUID          []byte
	SignCount       uint32
	Transports      []string
}
