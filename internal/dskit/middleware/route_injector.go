// SPDX-License-Identifier: AGPL-3.0-only

package middleware

import (
	"context"
	"net/http"
)

// Custom type to hide it from other packages.
type contextKey int

const contextKeyRouteName contextKey = 1

// RouteInjector is a middleware that injects the route name for the current request into the request context.
//
// The route name can be retrieved by calling ExtractRouteName.
type RouteInjector struct{}

func (i RouteInjector) Wrap(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routeName := getRouteName(r)
		handler.ServeHTTP(w, WithRouteName(r, routeName))
	})
}

// WithRouteName annotates r's context with the provided route name.
//
// This method should generally only be used in tests: in production code, use RouteInjector instead.
func WithRouteName(r *http.Request, routeName string) *http.Request {
	ctx := context.WithValue(r.Context(), contextKeyRouteName, routeName)
	return r.WithContext(ctx)
}

// ExtractRouteName returns the route name associated with this request that was previously injected by the
// RouteInjector middleware or WithRouteName.
//
// This is the same route name used for trace and metric names, and is already suitable for use as a Prometheus label
// value.
func ExtractRouteName(ctx context.Context) string {
	routeName, ok := ctx.Value(contextKeyRouteName).(string)
	if !ok {
		return ""
	}

	return routeName
}

func getRouteName(r *http.Request) string {
	return r.Pattern
}
