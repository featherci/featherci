package middleware

import "net/http"

// Chain applies middlewares to a handler in reverse order,
// so that the first middleware in the list is the outermost one.
func Chain(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	// Apply in reverse order so first middleware wraps outermost
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
