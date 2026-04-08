# FIDO2/WebAuthn Authentication Plugin Guide

## 🎯 Overview

The FIDO2 plugin provides **passwordless authentication** using WebAuthn/FIDO2 standards. Users can authenticate using:

- **Biometrics**: Touch ID, Face ID, Windows Hello
- **Hardware Keys**: YubiKey, SoloKey
- **Platform Authenticators**: Device built-in security keys
- **Cross-Platform**: USB, NFC, BLE security keys

**Benefits:**
✅ Phishing-resistant authentication  
✅ No passwords to manage or leak  
✅ Better user experience (one-tap login)  
✅ Industry standard (W3C WebAuthn)  
✅ Works on all modern browsers  

---

## 📋 Prerequisites

### Requirements

- **HTTPS** (required for production, localhost allowed for development)
- **Modern Browser**: Chrome 67+, Firefox 60+, Safari 13+, Edge 79+
- **Authenticator**: Device with biometric sensor or hardware security key
- **Go 1.25+** with Kopiochi project

### Dependencies

Already included in your project:
```
github.com/go-webauthn/webauthn v0.16.3
github.com/google/uuid v1.6.0
```

---

## 🔧 Setup

### 1. Enable the Plugin

Edit `config/default.yaml`:

```yaml
plugins:
  auth:
    fido2:
      enabled: true
      provider: fido2-auth
      config:
        rp_id: "localhost"                    # Your domain (no port)
        rp_origin: "http://localhost:3000"    # Frontend URL
        rp_name: "My Application"
        display_name: "My App"
```

### 2. Production Configuration

```yaml
plugins:
  auth:
    fido2:
      enabled: true
      provider: fido2-auth
      config:
        rp_id: "myapp.com"                    # Domain only
        rp_origin: "https://myapp.com"        # HTTPS required!
        rp_name: "My Application"
        display_name: "My App"
```

**Important:** `rp_origin` must match your frontend URL exactly (including protocol).

---

## 🏗️ Architecture

### Components

```
┌─────────────────────────────────────────────────┐
│              Your Frontend App                   │
│  (React, Vue, Vanilla JS, etc.)                 │
│                                                  │
│  navigator.credentials.create()  ← Register     │
│  navigator.credentials.get()     ← Login        │
└──────────────┬──────────────────────────────────┘
               │ HTTP + JSON
               ▼
┌─────────────────────────────────────────────────┐
│           Kopiochi API Server                    │
│                                                  │
│  FIDO2Handler                                   │
│  ├── BeginRegistration()  → PublicKeyOptions    │
│  ├── FinishRegistration() → Validate & Store    │
│  ├── BeginLogin()         → PublicKeyOptions    │
│  └── FinishLogin()        → Validate & Session  │
│                                                  │
│  FIDO2Plugin (internal/plugins/auth/fido2.go)   │
│  ├── UserStore    ← Your user database          │
│  └── SessionStore ← Session management           │
└──────────────┬──────────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────────┐
│         Your Database/Storage                    │
│                                                  │
│  Users Table                                    │
│  ├── id                                         │
│  ├── username                                   │
│  └── credentials (JSONB) ← FIDO2 credentials    │
│                                                  │
│  Sessions Table (optional)                      │
│  ├── session_id                                 │
│  ├── data (JSONB)                               │
│  └── expires_at                                 │
└─────────────────────────────────────────────────┘
```

### Registration Flow

```
User                    Frontend                  Backend (Kopiochi)
 │                       │                              │
 │──[Click Register]────>│                              │
 │                       │──POST /fido2/register/begin─>│
 │                       │                              │
 │                       │<─────publicKey options───────│
 │                       │        + session_id          │
 │                       │                              │
 │<──[Biometric Prompt]──│                              │
 │   (Touch ID/Face ID)  │                              │
 │                       │                              │
 │──[Credential Created]─>│                              │
 │                       │──POST /fido2/register/finish>│
 │                       │   + credential + session_id  │
 │                       │                              │
 │                       │<────Success + user_id────────│
 │                       │                              │
```

### Login Flow

```
User                    Frontend                  Backend (Kopiochi)
 │                       │                              │
 │──[Click Login]───────>│                              │
 │                       │──POST /fido2/login/begin────>│
 │                       │                              │
 │                       │<─────publicKey options───────│
 │                       │        + session_id          │
 │                       │                              │
 │<──[Biometric Prompt]──│                              │
 │   (Touch ID/Face ID)  │                              │
 │                       │                              │
 │──[Credential Found]───>│                              │
 │                       │──POST /fido2/login/finish───>│
 │                       │   + assertion + session_id   │
 │                       │                              │
 │                       │<──Success + session cookie───│
 │                       │                              │
```

---

## 💻 Implementation

### Step 1: Implement UserStore

The `UserStore` interface connects FIDO2 to your user database.

Create `internal/store/fido2_user_store.go`:

```go
package store

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"

    "github.com/go-webauthn/webauthn/webauthn"
    "github.com/sujanto-gaws/kopiochi/internal/plugins/auth"
)

type FIDO2UserStore struct {
    db *sql.DB
}

func NewFIDO2UserStore(db *sql.DB) *FIDO2UserStore {
    return &FIDO2UserStore{db: db}
}

// GetUserByID retrieves user with their FIDO2 credentials
func (s *FIDO2UserStore) GetUserByID(ctx context.Context, userID []byte) (auth.WebAuthnUser, error) {
    var user auth.WebAuthnUser
    
    var credentialsJSON []byte
    err := s.db.QueryRowContext(ctx,
        "SELECT id, username, display_name, credentials FROM users WHERE id = $1",
        userID,
    ).Scan(&user.ID, &user.Name, &user.DisplayName, &credentialsJSON)
    
    if err != nil {
        return user, fmt.Errorf("failed to fetch user: %w", err)
    }
    
    // Parse stored credentials
    if len(credentialsJSON) > 0 {
        if err := json.Unmarshal(credentialsJSON, &user.Credentials); err != nil {
            return user, fmt.Errorf("failed to parse credentials: %w", err)
        }
    }
    
    return user, nil
}

// GetUserByName finds user by username/email
func (s *FIDO2UserStore) GetUserByName(ctx context.Context, name string) (auth.WebAuthnUser, error) {
    var user auth.WebAuthnUser
    
    var credentialsJSON []byte
    err := s.db.QueryRowContext(ctx,
        "SELECT id, username, display_name, credentials FROM users WHERE username = $1",
        name,
    ).Scan(&user.ID, &user.Name, &user.DisplayName, &credentialsJSON)
    
    if err != nil {
        return user, fmt.Errorf("user not found: %w", err)
    }
    
    if len(credentialsJSON) > 0 {
        json.Unmarshal(credentialsJSON, &user.Credentials)
    }
    
    return user, nil
}

// SaveUser creates or updates user with FIDO2 credentials
func (s *FIDO2UserStore) SaveUser(ctx context.Context, user auth.WebAuthnUser) error {
    credentialsJSON, err := json.Marshal(user.Credentials)
    if err != nil {
        return fmt.Errorf("failed to marshal credentials: %w", err)
    }
    
    _, err = s.db.ExecContext(ctx,
        `INSERT INTO users (id, username, display_name, credentials) 
         VALUES ($1, $2, $3, $4)
         ON CONFLICT (id) DO UPDATE SET credentials = $4`,
        user.ID, user.Name, user.DisplayName, credentialsJSON,
    )
    
    return err
}

// GetUserByCredentialID finds user by FIDO2 credential ID
func (s *FIDO2UserStore) GetUserByCredentialID(ctx context.Context, credentialID []byte) (auth.WebAuthnUser, error) {
    // Query all users and check their credentials
    // (In production, use a more efficient query with JSONB operators)
    rows, err := s.db.QueryContext(ctx,
        "SELECT id, username, display_name, credentials FROM users",
    )
    if err != nil {
        return auth.WebAuthnUser{}, err
    }
    defer rows.Close()
    
    for rows.Next() {
        var user auth.WebAuthnUser
        var credentialsJSON []byte
        
        if err := rows.Scan(&user.ID, &user.Name, &user.DisplayName, &credentialsJSON); err != nil {
            continue
        }
        
        if len(credentialsJSON) == 0 {
            continue
        }
        
        var creds []webauthn.Credential
        if err := json.Unmarshal(credentialsJSON, &creds); err != nil {
            continue
        }
        
        for _, cred := range creds {
            if string(cred.ID) == string(credentialID) {
                user.Credentials = creds
                return user, nil
            }
        }
    }
    
    return auth.WebAuthnUser{}, fmt.Errorf("user not found for credential")
}
```

### Step 2: Create Database Schema

**PostgreSQL:**

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(255) UNIQUE NOT NULL,
    display_name VARCHAR(255),
    credentials JSONB,  -- Stores FIDO2 credentials array
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_users_username ON users(username);
```

### Step 3: Create FIDO2 Handler

Create `internal/infrastructure/http/handlers/fido2.go`:

```go
package handlers

import (
    "encoding/json"
    "net/http"

    "github.com/sujanto-gaws/kopiochi/internal/plugins/auth"
)

type FIDO2Handler struct {
    fido2Plugin *auth.FIDO2Plugin
}

func NewFIDO2Handler(fido2Plugin *auth.FIDO2Plugin) *FIDO2Handler {
    return &FIDO2Handler{fido2Plugin: fido2Plugin}
}

// BeginRegistration starts the FIDO2 registration process
// POST /api/v1/fido2/register/begin
func (h *FIDO2Handler) BeginRegistration() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req struct {
            UserID      string `json:"user_id"`
            UserName    string `json:"user_name"`
            DisplayName string `json:"display_name"`
        }

        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            auth.WriteJSON(w, http.StatusBadRequest, auth.FIDO2Error("invalid request body"))
            return
        }

        if req.UserID == "" || req.UserName == "" {
            auth.WriteJSON(w, http.StatusBadRequest, auth.FIDO2Error("user_id and user_name are required"))
            return
        }

        if req.DisplayName == "" {
            req.DisplayName = req.UserName
        }

        options, sessionID, err := h.fido2Plugin.BeginRegistration(
            r.Context(),
            req.UserID,
            req.UserName,
            req.DisplayName,
        )
        if err != nil {
            auth.WriteJSON(w, http.StatusInternalServerError, auth.FIDO2Error(err.Error()))
            return
        }

        auth.WriteJSON(w, http.StatusOK, map[string]interface{}{
            "options":    options,
            "session_id": sessionID,
        })
    }
}

// FinishRegistration completes the FIDO2 registration
// POST /api/v1/fido2/register/finish?session_id=xxx
func (h *FIDO2Handler) FinishRegistration() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        sessionID := r.URL.Query().Get("session_id")
        if sessionID == "" {
            auth.WriteJSON(w, http.StatusBadRequest, auth.FIDO2Error("session_id is required"))
            return
        }

        userID, err := h.fido2Plugin.FinishRegistration(r.Context(), sessionID, r)
        if err != nil {
            auth.WriteJSON(w, http.StatusBadRequest, auth.FIDO2Error(err.Error()))
            return
        }

        auth.WriteJSON(w, http.StatusOK, auth.FIDO2Success(map[string]string{
            "user_id": userID,
            "message": "Registration successful",
        }, ""))
    }
}

// BeginLogin starts the FIDO2 login process
// POST /api/v1/fido2/login/begin
func (h *FIDO2Handler) BeginLogin() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        options, sessionID, err := h.fido2Plugin.BeginLogin(r.Context())
        if err != nil {
            auth.WriteJSON(w, http.StatusInternalServerError, auth.FIDO2Error(err.Error()))
            return
        }

        auth.WriteJSON(w, http.StatusOK, map[string]interface{}{
            "options":    options,
            "session_id": sessionID,
        })
    }
}

// FinishLogin completes the FIDO2 login
// POST /api/v1/fido2/login/finish?session_id=xxx
func (h *FIDO2Handler) FinishLogin() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        sessionID := r.URL.Query().Get("session_id")
        if sessionID == "" {
            auth.WriteJSON(w, http.StatusBadRequest, auth.FIDO2Error("session_id is required"))
            return
        }

        userID, err := h.fido2Plugin.FinishLogin(r.Context(), sessionID, r)
        if err != nil {
            auth.WriteJSON(w, http.StatusUnauthorized, auth.FIDO2Error("login failed: "+err.Error()))
            return
        }

        // Set session cookie
        http.SetCookie(w, &http.Cookie{
            Name:     "fido2_session",
            Value:    sessionID,
            Path:     "/",
            HttpOnly: true,
            Secure:   r.TLS != nil, // true for HTTPS
            SameSite: http.SameSiteStrictMode,
            MaxAge:   86400, // 24 hours
        })

        auth.WriteJSON(w, http.StatusOK, auth.FIDO2Success(map[string]string{
            "user_id": userID,
            "message": "Login successful",
        }, sessionID))
    }
}

// GetUserInfo returns authenticated user info
// GET /api/v1/fido2/me
func (h *FIDO2Handler) GetUserInfo() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        userID := h.fido2Plugin.ExtractUserID(r.Context())
        if userID == "" {
            auth.WriteJSON(w, http.StatusUnauthorized, auth.FIDO2Error("not authenticated"))
            return
        }

        auth.WriteJSON(w, http.StatusOK, map[string]string{
            "user_id": userID,
        })
    }
}
```

### Step 4: Register Routes

Update `internal/infrastructure/http/routes/routes.go`:

```go
func Setup(r *chi.Mux, userHandler *handlers.UserHandler, fido2Handler *handlers.FIDO2Handler) {
    // Health check
    r.Get("/health", handlers.Health())

    // FIDO2 routes
    r.Route("/api/v1/fido2", func(r chi.Router) {
        // Registration
        r.Post("/register/begin", fido2Handler.BeginRegistration())
        r.Post("/register/finish", fido2Handler.FinishRegistration())
        
        // Login
        r.Post("/login/begin", fido2Handler.BeginLogin())
        r.Post("/login/finish", fido2Handler.FinishLogin())
        
        // User info (protected)
        r.Get("/me", fido2Handler.GetUserInfo())
    })
}
```

### Step 5: Initialize in main.go

Update `cmd/api/main.go`:

```go
// After plugin initialization
pluginRegistry := plugin.NewRegistry()
plugins.RegisterBuiltinPlugins(pluginRegistry)
plugin.InitializeFromConfig(pluginRegistry, &cfg.Plugins)

// Get FIDO2 plugin instance
fido2Plugin := pluginRegistry.GetAuth("fido2-auth")

if fido2Plugin != nil {
    // Initialize UserStore with your database
    userStore := store.NewFIDO2UserStore(pool) // Your DB pool
    
    // Re-initialize FIDO2 plugin with UserStore
    fido2Plugin.Initialize(map[string]interface{}{
        "rp_id":         "localhost",
        "rp_origin":     "http://localhost:3000",
        "rp_name":       "My App",
        "display_name":  "My Application",
        "user_store":    userStore,
    })
    
    // Create FIDO2 handler
    fido2Handler := handlers.NewFIDO2Handler(fido2Plugin.(*auth.FIDO2Plugin))
    
    // Setup routes with FIDO2 handler
    routes.Setup(r, userHandler, fido2Handler)
}
```

---

## 🌐 Frontend Integration

### Vanilla JavaScript

Create `frontend/fido2.js`:

```javascript
class FIDO2Auth {
    constructor(apiBaseUrl) {
        this.apiUrl = apiBaseUrl;
    }

    // Helper: Convert base64url to Uint8Array
    _base64urlToBuffer(base64url) {
        const padding = '='.repeat((4 - (base64url.length % 4)) % 4);
        const base64 = (base64url + padding).replace(/-/g, '+').replace(/_/g, '/');
        const rawData = atob(base64);
        const outputArray = new Uint8Array(rawData.length);
        for (let i = 0; i < rawData.length; ++i) {
            outputArray[i] = rawData.charCodeAt(i);
        }
        return outputArray;
    }

    // Helper: Convert Uint8Array to base64url
    _bufferToBase64url(buffer) {
        let binary = '';
        const bytes = new Uint8Array(buffer);
        for (let i = 0; i < bytes.byteLength; i++) {
            binary += String.fromCharCode(bytes[i]);
        }
        return btoa(binary)
            .replace(/\+/g, '-')
            .replace(/\//g, '_')
            .replace(/=+$/, '');
    }

    // Register a new passkey
    async register(userId, userName, displayName) {
        try {
            // Step 1: Begin registration
            const beginRes = await fetch(`${this.apiUrl}/fido2/register/begin`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    user_id: userId,
                    user_name: userName,
                    display_name: displayName || userName,
                }),
            });

            const beginData = await beginRes.json();
            if (!beginData.options) {
                throw new Error(beginData.error || 'Registration failed');
            }

            // Step 2: Create credential
            const publicKey = this._preparePublicKey(beginData.options.publicKey);
            const credential = await navigator.credentials.create({ publicKey });

            // Step 3: Finish registration
            const finishRes = await fetch(
                `${this.apiUrl}/fido2/register/finish?session_id=${beginData.session_id}`,
                {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(this._credentialToJSON(credential)),
                }
            );

            const finishData = await finishRes.json();
            if (!finishData.success) {
                throw new Error(finishData.error);
            }

            return finishData;
        } catch (error) {
            console.error('Registration error:', error);
            throw error;
        }
    }

    // Login with passkey
    async login() {
        try {
            // Step 1: Begin login
            const beginRes = await fetch(`${this.apiUrl}/fido2/login/begin`, {
                method: 'POST',
            });

            const beginData = await beginRes.json();
            if (!beginData.options) {
                throw new Error(beginData.error || 'Login failed');
            }

            // Step 2: Get credential
            const publicKey = this._preparePublicKey(beginData.options.publicKey);
            const credential = await navigator.credentials.get({ publicKey });

            // Step 3: Finish login
            const finishRes = await fetch(
                `${this.apiUrl}/fido2/login/finish?session_id=${beginData.session_id}`,
                {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(this._credentialToJSON(credential)),
                }
            );

            const finishData = await finishRes.json();
            if (!finishData.success) {
                throw new Error(finishData.error);
            }

            return finishData;
        } catch (error) {
            console.error('Login error:', error);
            throw error;
        }
    }

    // Convert server options to WebAuthn format
    _preparePublicKey(options) {
        return {
            ...options,
            challenge: this._base64urlToBuffer(options.challenge),
            user: {
                ...options.user,
                id: this._base64urlToBuffer(options.user.id),
            },
            excludeCredentials: options.excludeCredentials?.map(cred => ({
                ...cred,
                id: this._base64urlToBuffer(cred.id),
            })),
            allowCredentials: options.allowCredentials?.map(cred => ({
                ...cred,
                id: this._base64urlToBuffer(cred.id),
            })),
        };
    }

    // Convert credential to JSON
    _credentialToJSON(cred) {
        return {
            id: cred.id,
            rawId: this._bufferToBase64url(cred.rawId),
            type: cred.type,
            response: {
                clientDataJSON: this._bufferToBase64url(cred.response.clientDataJSON),
                attestationObject: cred.response.attestationObject 
                    ? this._bufferToBase64url(cred.response.attestationObject)
                    : undefined,
                authenticatorData: cred.response.authenticatorData
                    ? this._bufferToBase64url(cred.response.authenticatorData)
                    : undefined,
                signature: cred.response.signature
                    ? this._bufferToBase64url(cred.response.signature)
                    : undefined,
                userHandle: cred.response.userHandle
                    ? this._bufferToBase64url(cred.response.userHandle)
                    : undefined,
            },
        };
    }
}

// Usage
const fido2 = new FIDO2Auth('http://localhost:8080/api/v1');

// Register
document.getElementById('register-btn').addEventListener('click', async () => {
    try {
        const result = await fido2.register(
            'user-123',
            'john@example.com',
            'John Doe'
        );
        alert('Registration successful!');
    } catch (error) {
        alert('Registration failed: ' + error.message);
    }
});

// Login
document.getElementById('login-btn').addEventListener('click', async () => {
    try {
        const result = await fido2.login();
        alert('Login successful! User: ' + result.data.user_id);
        // Redirect to dashboard
        window.location.href = '/dashboard';
    } catch (error) {
        alert('Login failed: ' + error.message);
    }
});
```

### React Example

```jsx
import { useState } from 'react';

function FIDO2Login() {
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    const handleLogin = async () => {
        setLoading(true);
        setError(null);

        try {
            // Begin login
            const beginRes = await fetch('/api/v1/fido2/login/begin', {
                method: 'POST',
            });
            const beginData = await beginRes.json();

            if (!beginData.options) {
                throw new Error(beginData.error);
            }

            // Get credential
            const credential = await navigator.credentials.get({
                publicKey: preparePublicKey(beginData.options.publicKey),
            });

            // Finish login
            const finishRes = await fetch(
                `/api/v1/fido2/login/finish?session_id=${beginData.session_id}`,
                {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(credentialToJSON(credential)),
                }
            );

            const finishData = await finishRes.json();
            if (!finishData.success) {
                throw new Error(finishData.error);
            }

            // Redirect on success
            window.location.href = '/dashboard';
        } catch (err) {
            setError(err.message);
        } finally {
            setLoading(false);
        }
    };

    return (
        <div>
            <h2>Login with Passkey</h2>
            {error && <div style={{ color: 'red' }}>{error}</div>}
            <button onClick={handleLogin} disabled={loading}>
                {loading ? 'Authenticating...' : 'Login with Passkey'}
            </button>
        </div>
    );
}
```

---

## 🔒 Security Best Practices

### 1. Always Use HTTPS in Production

```yaml
# ❌ Wrong (production)
rp_origin: "http://myapp.com"

# ✅ Correct
rp_origin: "https://myapp.com"
```

### 2. Secure Session Cookies

```go
http.SetCookie(w, &http.Cookie{
    Name:     "fido2_session",
    HttpOnly: true,      // Prevent JavaScript access
    Secure:   true,       // HTTPS only
    SameSite: http.SameSiteStrictMode,  // CSRF protection
    MaxAge:   86400,      // 24 hours
})
```

### 3. Validate User Existence

Before registration, ensure user exists in your system:

```go
func (h *FIDO2Handler) BeginRegistration() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Verify user exists first
        user, err := h.userStore.GetUserByName(r.Context(), req.UserName)
        if err != nil {
            auth.WriteJSON(w, http.StatusNotFound, auth.FIDO2Error("user not found"))
            return
        }
        
        // Then proceed with FIDO2 registration
    }
}
```

### 4. Implement Session Validation

Create middleware to check FIDO2 session:

```go
func FIDO2AuthMiddleware(fido2Plugin *auth.FIDO2Plugin) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            userID := fido2Plugin.ExtractUserID(r.Context())
            if userID == "" {
                auth.WriteJSON(w, http.StatusUnauthorized, auth.FIDO2Error("authentication required"))
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

// Usage
r.With(FIDO2AuthMiddleware(fido2Plugin)).Get("/api/v1/protected", handler)
```

### 5. Rate Limiting

Combine with rate limiting plugin:

```yaml
plugins:
  middleware:
    - ratelimit
    - cors
  
  auth:
    fido2:
      enabled: true
      provider: fido2-auth
      config:
        rp_id: "myapp.com"
        rp_origin: "https://myapp.com"
```

---

## 🧪 Testing

### Test with Local Development

```bash
# 1. Start your API server
make run

# 2. Open browser to frontend
open http://localhost:3000

# 3. Test registration
# - Click "Register with Passkey"
# - Browser will prompt for biometric/PIN
# - Complete the prompt

# 4. Test login
# - Click "Login with Passkey"
# - Browser will prompt for same biometric/PIN
# - You should be logged in
```

### Test with YubiKey

1. Insert YubiKey via USB
2. When prompted, touch the YubiKey button
3. Registration/login will complete

### Test Without Hardware

**Windows Hello:**
- Settings > Accounts > Sign-in options
- Set up Windows Hello

**Touch ID (Mac):**
- System Preferences > Touch ID
- Add fingerprint

**Face ID (iPhone/Mac):**
- Already configured in iCloud Keychain

---

## 🐛 Troubleshooting

### Error: "rp_origin is required"

**Problem:** Configuration missing `rp_origin`.

**Solution:**
```yaml
config:
  rp_id: "localhost"
  rp_origin: "http://localhost:3000"  # ← Add this
```

### Error: "user_store is required"

**Problem:** FIDO2 plugin needs a UserStore implementation.

**Solution:** Implement the `UserStore` interface and pass it during initialization:

```go
userStore := store.NewFIDO2UserStore(db)
fido2Plugin.Initialize(map[string]interface{}{
    "rp_id":       "localhost",
    "rp_origin":   "http://localhost:3000",
    "user_store":  userStore,  // ← Add this
})
```

### Error: "navigator.credentials is undefined"

**Cause:** 
- Browser doesn't support WebAuthn
- Page not served over HTTPS (or localhost)

**Solution:**
- Use a modern browser (Chrome, Firefox, Safari, Edge)
- Use `http://localhost` for development (special case)
- Use `https://` in production

### Registration Succeeds but Login Fails

**Cause:** Credentials not saved properly.

**Debug:**
```sql
-- Check if credentials were saved
SELECT id, username, credentials FROM users WHERE username = 'john@example.com';

-- Should show JSONB array with credential data
```

### "User handle is required" Error

**Cause:** User ID not properly set during registration.

**Solution:** Ensure user ID is a byte slice, not empty:

```go
user := &auth.WebAuthnUser{
    ID: []byte(userID),  // ← Must be set!
    Name: userName,
}
```

---

## 📊 Browser Support

| Browser | Version | Platform |
|---------|---------|----------|
| Chrome | 67+ | All |
| Firefox | 60+ | All |
| Safari | 13+ | macOS, iOS |
| Edge | 79+ | Windows |
| Samsung Internet | 13+ | Android |

**Mobile Support:**
- iOS 13.3+ (Safari)
- Android 7+ (Chrome)

---

## 🔗 Additional Resources

- [W3C WebAuthn Specification](https://www.w3.org/TR/webauthn-2/)
- [FIDO Alliance](https://fidoalliance.org/)
- [go-webauthn Documentation](https://pkg.go.dev/github.com/go-webauthn/webauthn)
- [MDN WebAuthn Guide](https://developer.mozilla.org/en-US/docs/Web/API/Web_Authentication_API)
- [Passkeys Guide (Google)](https://developers.google.com/identity/fido/overview)

---

## 📝 Summary

**FIDO2 plugin provides:**
✅ Passwordless authentication  
✅ Phishing-resistant security  
✅ Biometric support (Touch ID, Face ID, Windows Hello)  
✅ Hardware key support (YubiKey)  
✅ Industry standard implementation  

**Setup checklist:**
- [ ] Enable plugin in config
- [ ] Implement UserStore interface
- [ ] Create database schema
- [ ] Create FIDO2 handler
- [ ] Register routes
- [ ] Add frontend integration
- [ ] Test registration flow
- [ ] Test login flow
- [ ] Add session validation middleware
- [ ] Deploy with HTTPS

For questions or issues, see [PLUGIN_GUIDE.md](PLUGIN_GUIDE.md) or open a GitHub issue.
