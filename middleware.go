//go:build tailscale

package tailkit

import (
	"context"
	"net/http"
)

// CallerContextKey is the exported context key type for CallerIdentity.
type CallerContextKey struct{}

// callerKey aliases CallerContextKey so server.go's withCallerIdentity compiles.
type callerKey = CallerContextKey

// CallerIdentity holds the verified identity of the caller on an inbound request.
type CallerIdentity struct {
	Hostname    string
	TailscaleIP string
	UserLogin   string
	Caps        map[string]bool
}

// HasCap reports whether the caller was granted the given ACL capability.
func (id CallerIdentity) HasCap(cap string) bool {
	return id.Caps[cap]
}

// CallerFromContext retrieves the CallerIdentity injected by AuthMiddleware.
func CallerFromContext(ctx context.Context) (CallerIdentity, bool) {
	id, ok := ctx.Value(CallerContextKey{}).(CallerIdentity)
	return id, ok
}

// AuthMiddleware authenticates every inbound request via Tailscale's WhoIs API.
func AuthMiddleware(srv *Server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lc := srv.localClient()
			if lc == nil {
				http.Error(w, "tailkit: local client unavailable", http.StatusInternalServerError)
				return
			}

			who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
			if err != nil {
				http.Error(w, "tailkit: unauthorized — not a tailnet peer", http.StatusUnauthorized)
				return
			}

			id := CallerIdentity{Caps: make(map[string]bool)}

			// who.Node is *tailcfg.Node — plain pointer, plain field access.
			if who.Node != nil {
				id.Hostname = who.Node.ComputedName
				if len(who.Node.Addresses) > 0 {
					id.TailscaleIP = who.Node.Addresses[0].Addr().String()
				}
			}
			if who.UserProfile != nil {
				id.UserLogin = who.UserProfile.LoginName
			}
			for capURL := range who.CapMap {
				id.Caps[string(capURL)] = true
			}

			ctx := context.WithValue(r.Context(), CallerContextKey{}, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
