// Package ctx stores the process-wide Wails application context.
package ctx

import "context"

var applicationContext context.Context

// SetContext stores the Wails application context during startup.
// Calling SetContext more than once is a programming error.
func SetContext(value context.Context) {
	if value == nil {
		panic("backend context: cannot set a nil context")
	}
	if applicationContext != nil {
		panic("backend context: context is already initialized")
	}
	applicationContext = value
}

// GetContext returns the Wails application context.
// Calling it before startup is a programming error.
func GetContext() context.Context {
	if applicationContext == nil {
		panic("backend context: context is not initialized")
	}
	return applicationContext
}

// HasWailsEvents reports whether the context came from the Wails lifecycle.
func HasWailsEvents() bool {
	return applicationContext != nil && applicationContext.Value("events") != nil
}
