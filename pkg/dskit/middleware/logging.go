// Provenance-includes-location: https://github.com/weaveworks/common/blob/main/middleware/logging.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Weaveworks Ltd.

package middleware

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/tracing"
)

// Log middleware logs http requests
type Log struct {
	Log                      *slog.Logger
	DisableRequestSuccessLog bool
	LogRequestHeaders        bool // LogRequestHeaders true -> dump http headers at debug log level
	LogRequestAtInfoLevel    bool // LogRequestAtInfoLevel true -> log requests at info log level
	SourceIPs                *SourceIPExtractor
	HTTPHeadersToExclude     map[string]bool
}

var defaultExcludedHeaders = map[string]bool{
	"Cookie":        true,
	"X-Csrf-Token":  true,
	"Authorization": true,
}

func NewLogMiddleware(log *slog.Logger, logRequestHeaders bool, logRequestAtInfoLevel bool, sourceIPs *SourceIPExtractor, headersList []string) Log {
	httpHeadersToExclude := map[string]bool{}
	for header := range defaultExcludedHeaders {
		httpHeadersToExclude[header] = true
	}
	for _, header := range headersList {
		httpHeadersToExclude[header] = true
	}

	return Log{
		Log:                   log,
		LogRequestHeaders:     logRequestHeaders,
		LogRequestAtInfoLevel: logRequestAtInfoLevel,
		SourceIPs:             sourceIPs,
		HTTPHeadersToExclude:  httpHeadersToExclude,
	}
}

// logWithRequest information from the request and context as fields.
func (l Log) logWithRequest(r *http.Request) *slog.Logger {
	localLog := l.Log
	traceID, ok := tracing.ExtractSampledTraceID(r.Context())
	if ok {
		localLog = localLog.With("traceID", traceID)
	} else if traceID != "" {
		localLog = localLog.With("traceIDUnsampled", traceID)
	}

	if l.SourceIPs != nil {
		ips := l.SourceIPs.Get(r)
		if ips != "" {
			localLog = localLog.With("sourceIPs", ips)
		}
	}

	return localLog
}

// Wrap implements Middleware
func (l Log) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		uri := r.RequestURI // capture the URI before running next, as it may get rewritten
		requestLog := l.logWithRequest(r)
		// Log headers before running 'next' in case other interceptors change the data.
		headers, err := dumpRequest(r, l.HTTPHeadersToExclude)
		if err != nil {
			headers = nil
			l.Log.Error("could not dump request headers", "err", err)
		}
		var buf bytes.Buffer
		wrapped := newBadResponseLoggingWriter(w, &buf)
		next.ServeHTTP(wrapped, r)

		statusCode, writeErr := wrapped.getStatusCode(), wrapped.getWriteError()

		if writeErr != nil {
			if errors.Is(writeErr, context.Canceled) {
				if l.LogRequestAtInfoLevel {
					if l.LogRequestHeaders && headers != nil {
						requestLog.Info("request cancelled",
							"method", r.Method,
							"uri", uri,
							"duration", time.Since(begin),
							"err", writeErr,
							"isWSHandshakeRequest", IsWSHandshakeRequest(r),
							"headers", string(headers),
						)
					} else {
						requestLog.Info("request cancelled",
							"method", r.Method,
							"uri", uri,
							"duration", time.Since(begin),
							"err", writeErr,
							"isWSHandshakeRequest", IsWSHandshakeRequest(r),
						)
					}
				} else {
					if l.LogRequestHeaders && headers != nil {
						requestLog.Debug("request cancelled",
							"method", r.Method,
							"uri", uri,
							"duration", time.Since(begin),
							"err", writeErr,
							"isWSHandshakeRequest", IsWSHandshakeRequest(r),
							"headers", string(headers),
						)
					} else {
						requestLog.Debug("request cancelled",
							"method", r.Method,
							"uri", uri,
							"duration", time.Since(begin),
							"err", writeErr,
							"isWSHandshakeRequest", IsWSHandshakeRequest(r),
						)
					}
				}
			} else {
				if l.LogRequestHeaders && headers != nil {
					requestLog.Warn("error",
						"method", r.Method,
						"uri", uri,
						"duration", time.Since(begin),
						"err", writeErr,
						"isWSHandshakeRequest", IsWSHandshakeRequest(r),
						"headers", string(headers),
					)
				} else {
					requestLog.Warn("error",
						"method", r.Method,
						"uri", uri,
						"duration", time.Since(begin),
						"err", writeErr,
						"isWSHandshakeRequest", IsWSHandshakeRequest(r),
					)
				}
			}
			return
		}

		switch {
		// success and shouldn't log successful requests.
		case statusCode >= 200 && statusCode < 300 && l.DisableRequestSuccessLog:
			return

		case 100 <= statusCode && statusCode < 500 || statusCode == http.StatusBadGateway || statusCode == http.StatusServiceUnavailable:
			if l.LogRequestAtInfoLevel {
				if l.LogRequestHeaders && headers != nil {
					requestLog.Info("request",
						"method", r.Method,
						"uri", uri,
						"statusCode", statusCode,
						"duration", time.Since(begin),
						"isWSHandshakeRequest", IsWSHandshakeRequest(r),
						"headers", string(headers),
					)
				} else {
					requestLog.Info("request",
						"method", r.Method,
						"uri", uri,
						"statusCode", statusCode,
						"duration", time.Since(begin),
					)
				}
			} else {
				if l.LogRequestHeaders && headers != nil {
					requestLog.Debug("request",
						"method", r.Method,
						"uri", uri,
						"statusCode", statusCode,
						"duration", time.Since(begin),
						"isWSHandshakeRequest", IsWSHandshakeRequest(r),
						"headers", string(headers),
					)
				} else {
					requestLog.Debug("request",
						"method", r.Method,
						"uri", uri,
						"statusCode", statusCode,
						"duration", time.Since(begin),
					)
				}
			}
		default:
			if l.LogRequestHeaders && headers != nil {
				requestLog.Warn("request",
					"method", r.Method,
					"uri", uri,
					"statusCode", statusCode,
					"duration", time.Since(begin),
					"response", buf.Bytes(),
					"isWSHandshakeRequest", IsWSHandshakeRequest(r),
					"headers", string(headers),
				)
			} else {
				requestLog.Warn("request",
					"method", r.Method,
					"uri", uri,
					"statusCode", statusCode,
					"duration", time.Since(begin),
					"response", buf.Bytes(),
				)
			}
		}
	})
}

func dumpRequest(req *http.Request, httpHeadersToExclude map[string]bool) ([]byte, error) {
	var b bytes.Buffer

	// In case users initialize the Log middleware using the exported struct, skip the default headers anyway
	if len(httpHeadersToExclude) == 0 {
		httpHeadersToExclude = defaultExcludedHeaders
	}
	// Exclude some headers for security, or just that we don't need them when debugging
	err := req.Header.WriteSubset(&b, httpHeadersToExclude)
	if err != nil {
		return nil, err
	}

	ret := bytes.ReplaceAll(b.Bytes(), []byte("\r\n"), []byte("; "))
	return ret, nil
}

// IsWSHandshakeRequest returns true if the given request is a websocket handshake request.
func IsWSHandshakeRequest(req *http.Request) bool {
	if strings.ToLower(req.Header.Get("Upgrade")) == "websocket" {
		// Connection header values can be of form "foo, bar, ..."
		parts := strings.Split(strings.ToLower(req.Header.Get("Connection")), ",")
		for _, part := range parts {
			if strings.TrimSpace(part) == "upgrade" {
				return true
			}
		}
	}
	return false
}
