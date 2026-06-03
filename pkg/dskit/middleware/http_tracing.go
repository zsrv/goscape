// Provenance-includes-location: https://github.com/weaveworks/common/blob/main/middleware/http_tracing.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Weaveworks Ltd.

package middleware

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
)

// Tracer is a middleware which traces incoming requests.
type Tracer struct {
	SourceIPs *SourceIPExtractor
}

// Wrap implements Interface
func (t Tracer) Wrap(next http.Handler) http.Handler {
	tracingMiddleware := otelhttp.NewHandler(next, "http.tracing", otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		return httpOperationName(r)
	}))

	// Wrap the 'tracingMiddleware' to capture its execution
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
			// add a tag with the client's user agent to the span
			userAgent := r.Header.Get("User-Agent")
			if userAgent != "" {
				labeler.Add(attribute.String("http.user_agent", userAgent))
			}

			labeler.Add(attribute.String("http.url", r.URL.Path))
			labeler.Add(attribute.String("http.method", r.Method))

			// add the content type, useful when query requests are sent as POST
			if ct := r.Header.Get("Content-Type"); ct != "" {
				labeler.Add(attribute.String("http.content_type", ct))
			}

			labeler.Add(attribute.String("headers", fmt.Sprintf("%v", r.Header)))
			// add a tag with the client's sourceIPs to the span, if a
			// SourceIPExtractor is given.
			if t.SourceIPs != nil {
				labeler.Add(attribute.String("sourceIPs", t.SourceIPs.Get(r)))
			}
		}

		tracingMiddleware.ServeHTTP(w, r)
	})

	return handler
}

func httpOperationName(r *http.Request) string {
	routeName := ExtractRouteName(r.Context())
	return getOperationName(routeName, r)
}

func getOperationName(routeName string, r *http.Request) string {
	if routeName == "" {
		return "HTTP " + r.Method
	}
	return fmt.Sprintf("HTTP %s - %s", r.Method, routeName)
}
