package auth

import (
    "crypto/sha256"
    "encoding/hex"
    "time"
)

type RefreshToken struct {
    UserID    string
    TokenHash string
    ExpiresAt time.Time
}

func HashToken(plain string) string {
    h := sha256.Sum256([]byte(plain))
    return hex.EncodeToString(h[:])
}
