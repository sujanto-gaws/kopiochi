package plugin

import (
	"net/http"
	"sync"
)

// MiddlewareChain builds and manages a chain of HTTP middleware from plugins.
type MiddlewareChain struct {
	mu         sync.RWMutex
	middleware []func(http.Handler) http.Handler
}

// NewMiddlewareChain creates a new empty middleware chain.
func NewMiddlewareChain() *MiddlewareChain {
	return &MiddlewareChain{
		middleware: make([]func(http.Handler) http.Handler, 0),
	}
}

// NewMiddlewareChainFromRegistry creates a middleware chain from a list of plugin names.
// Only plugins that implement MiddlewarePlugin will be added.
func NewMiddlewareChainFromRegistry(registry *Registry, pluginNames []string) *MiddlewareChain {
	chain := NewMiddlewareChain()
	
	for _, name := range pluginNames {
		mwPlugin := registry.GetMiddleware(name)
		if mwPlugin != nil {
			chain.Append(mwPlugin.Middleware())
		}
	}
	
	return chain
}

// Append adds a middleware function to the chain.
func (mc *MiddlewareChain) Append(mw func(http.Handler) http.Handler) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	mc.middleware = append(mc.middleware, mw)
}

// AppendMultiple adds multiple middleware functions to the chain.
func (mc *MiddlewareChain) AppendMultiple(mws ...func(http.Handler) http.Handler) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	mc.middleware = append(mc.middleware, mws...)
}

// Build wraps the final handler with all middleware in the chain.
// Middleware is applied in order: first added = outermost wrapper.
func (mc *MiddlewareChain) Build(final http.Handler) http.Handler {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	handler := final
	
	// Apply in reverse order so first added is outermost
	for i := len(mc.middleware) - 1; i >= 0; i-- {
		handler = mc.middleware[i](handler)
	}
	
	return handler
}

// Len returns the number of middleware in the chain.
func (mc *MiddlewareChain) Len() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	return len(mc.middleware)
}

// Clear removes all middleware from the chain.
func (mc *MiddlewareChain) Clear() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	mc.middleware = make([]func(http.Handler) http.Handler, 0)
}
