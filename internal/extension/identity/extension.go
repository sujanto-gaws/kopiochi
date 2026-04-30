package identity

import (
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/sujanto-gaws/kopiochi/internal/extension"
)

// IdentityExtension implements the Extension interface for identity management
type IdentityExtension struct {
	*extension.BaseExtension
	db      bun.IDB
	repo    Repository
	service Service
}

// NewIdentityExtension creates a new identity extension instance
func NewIdentityExtension() *IdentityExtension {
	return &IdentityExtension{
		BaseExtension: extension.NewBaseExtension("identity"),
	}
}

// Init initializes the identity extension with configuration
func (e *IdentityExtension) Init(config map[string]interface{}) error {
	if err := e.BaseExtension.Init(config); err != nil {
		return err
	}

	// Validate required configuration
	if config["db"] == nil {
		return fmt.Errorf("identity: db connection is required")
	}

	db, ok := config["db"].(bun.IDB)
	if !ok {
		return fmt.Errorf("identity: db must implement bun.IDB interface")
	}
	e.db = db

	// Optional configuration with defaults
	maxFailedAccesses := 5
	if val, ok := config["max_failed_accesses"]; ok {
		switch v := val.(type) {
		case int:
			maxFailedAccesses = v
		case int64:
			maxFailedAccesses = int(v)
		case float64:
			maxFailedAccesses = int(v)
		}
	}

	lockoutDuration := 15 // minutes
	if val, ok := config["lockout_duration_minutes"]; ok {
		switch v := val.(type) {
		case int:
			lockoutDuration = v
		case int64:
			lockoutDuration = int(v)
		case float64:
			lockoutDuration = int(v)
		}
	}

	// Initialize repository and service
	e.repo = NewRepository(e.db)
	e.service = NewService(e.repo, ServiceConfig{
		MaxFailedAccesses: maxFailedAccesses,
		LockoutDuration:   time.Duration(lockoutDuration) * time.Minute,
	})

	return nil
}

// Bootstrap is called during application bootstrap
func (e *IdentityExtension) Bootstrap(app extension.Application) error {
	// Register the service in the application container
	app.RegisterService("identity_repository", func() interface{} { return e.repo })
	app.RegisterService("identity_service", func() interface{} { return e.service })

	app.Logger().Info("Identity extension bootstrapped", "component", "identity")
	return nil
}

// Shutdown is called when the application shuts down
func (e *IdentityExtension) Shutdown() error {
	// Cleanup if needed
	e.db = nil
	e.repo = nil
	e.service = nil
	return nil
}

// GetRepository returns the identity repository
func (e *IdentityExtension) GetRepository() Repository {
	return e.repo
}

// GetService returns the identity service
func (e *IdentityExtension) GetService() Service {
	return e.service
}

// Ensure IdentityExtension implements the Extension interface
var _ extension.Extension = (*IdentityExtension)(nil)
