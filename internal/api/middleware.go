package api

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
)

// maxBodyBytes caps request bodies so a client can't make the server
// read an unbounded amount of data into memory.
const maxBodyBytes = 1 << 16 // 64 KiB

// allowedHosts builds the set of Host header values this single-user,
// no-login app will accept, for the given port.
func allowedHosts(port int) map[string]bool {
	return map[string]bool{
		fmt.Sprintf("localhost:%d", port): true,
		fmt.Sprintf("127.0.0.1:%d", port): true,
		fmt.Sprintf("[::1]:%d", port):     true,
	}
}

// protect guards against DNS-rebinding and cross-origin browser
// requests: even though the server only listens on loopback, a page
// on another domain can trick a browser into resolving its hostname
// to 127.0.0.1, so the Host (and Origin, if present) must also match.
// No CORS headers are ever sent, so browsers can't read responses from
// another origin either way.
func protect(hosts map[string]bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !hosts[r.Host] {
				writeError(w, http.StatusForbidden, "unrecognized host")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || !hosts[u.Host] {
					writeError(w, http.StatusForbidden, "unrecognized origin")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireJSON rejects POST/PUT requests that don't declare a JSON
// body, and caps every request body's size.
func requireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			ct := r.Header.Get("Content-Type")
			mediaType, _, err := mime.ParseMediaType(ct)
			if err != nil || mediaType != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
