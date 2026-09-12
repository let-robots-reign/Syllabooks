package handlers

import (
	"cmp"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"syllabooks/db/gen"
)

// OAuth login (PRD §8), shared by Yandex and VK ID. The browser goes to
// /api/auth/{provider}, which sends it on to the provider. The provider sends
// it back to /api/auth/{provider}/callback, which signs the user in and hands
// the session token to the frontend in the fragment of /auth/callback#token=….
// A fragment never reaches a server log or a Referer header, and the frontend
// strips it from the address bar straight away.
//
// Both providers use PKCE: VK ID requires it, and Yandex supports it.

// Provider is an OAuth login provider. NewYandex and NewVK build the real ones.
type Provider struct {
	// Name is both the users.oauth_provider value and the URL segment.
	Name  string
	OAuth oauth2.Config
	// ProfileURL returns the signed-in user's profile. A field so tests can
	// point it at a fake provider.
	ProfileURL string

	// echoParams are callback query parameters the provider wants repeated in
	// the token request.
	echoParams []string
	// fetchProfile reads the user's profile with an access token.
	fetchProfile func(ctx context.Context, p *Provider, accessToken string) (oauthProfile, error)
}

// oauthProfile is what signing in needs from a provider.
type oauthProfile struct {
	subject string // the provider's stable user ID
	name    string
}

const flowCookieName = "oauth_flow"

// flowCookie carries the state and the PKCE verifier from the start of a login
// to its callback. The state ties the callback to this browser; the verifier
// proves to the provider that whoever redeems the code started the login.
// SameSite=Lax still sends it on the provider's top-level redirect back.
func (s *Server) flowCookie(p *Provider, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     flowCookieName,
		Value:    value,
		Path:     "/api/auth/" + p.Name,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	}
}

const providerNotConfigured = "Этот способ входа пока не настроен."

// oauthStart sends the browser to p's consent screen.
func (s *Server) oauthStart(p *Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p == nil {
			writeError(w, http.StatusServiceUnavailable, providerNotConfigured)
			return
		}
		// Both are base64url, so a dot can separate them.
		state, verifier := newToken(), oauth2.GenerateVerifier()
		http.SetCookie(w, s.flowCookie(p, state+"."+verifier, 10*60))
		http.Redirect(w, r, p.OAuth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusFound)
	}
}

// oauthCallback is where p sends the browser back to.
func (s *Server) oauthCallback(p *Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p == nil {
			writeError(w, http.StatusServiceUnavailable, providerNotConfigured)
			return
		}
		// The state and verifier are single-use, whatever happens next.
		http.SetCookie(w, s.flowCookie(p, "", -1))

		query := r.URL.Query()
		if query.Get("error") != "" {
			// Usually access_denied: the student cancelled on the consent screen.
			redirectToApp(w, r, "error=cancelled")
			return
		}
		var state, verifier string
		if cookie, err := r.Cookie(flowCookieName); err == nil {
			state, verifier, _ = strings.Cut(cookie.Value, ".")
		}
		if state == "" || verifier == "" ||
			subtle.ConstantTimeCompare([]byte(state), []byte(query.Get("state"))) != 1 {
			log.Printf("%s callback: state does not match the cookie", p.Name)
			redirectToApp(w, r, "error=failed")
			return
		}

		user, err := s.oauthUser(r.Context(), p, query, verifier)
		if err != nil {
			log.Printf("%s callback: %v", p.Name, err)
			redirectToApp(w, r, "error=failed")
			return
		}
		if user.Status == gen.UserStatusBanned {
			redirectToApp(w, r, "error=banned")
			return
		}
		token, err := s.startSession(r.Context(), user.ID)
		if err != nil {
			log.Printf("%s callback: %v", p.Name, err)
			redirectToApp(w, r, "error=failed")
			return
		}
		redirectToApp(w, r, "token="+token)
	}
}

// oauthUser redeems the callback's code for an access token, reads the
// profile with it, and finds or creates the matching user. The access token
// is used once and never stored.
func (s *Server) oauthUser(ctx context.Context, p *Provider, callback url.Values, verifier string) (gen.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	opts := []oauth2.AuthCodeOption{oauth2.VerifierOption(verifier)}
	for _, name := range p.echoParams {
		opts = append(opts, oauth2.SetAuthURLParam(name, callback.Get(name)))
	}
	token, err := p.OAuth.Exchange(ctx, callback.Get("code"), opts...)
	if err != nil {
		return gen.User{}, fmt.Errorf("exchange code: %w", err)
	}
	profile, err := p.fetchProfile(ctx, p, token.AccessToken)
	if err != nil {
		return gen.User{}, fmt.Errorf("fetch profile: %w", err)
	}
	if profile.subject == "" {
		return gen.User{}, errors.New("profile has no user ID")
	}

	user, err := gen.New(s.Pool).UpsertOAuthUser(ctx, gen.UpsertOAuthUserParams{
		OauthProvider: p.Name,
		OauthSubject:  profile.subject,
		// Only used when creating the user; the teacher corrects it from there.
		DisplayName: cmp.Or(profile.name, "Без имени"),
	})
	if err != nil {
		return gen.User{}, fmt.Errorf("upsert user: %w", err)
	}
	return user, nil
}

// fetchJSON sends req and decodes a 200 JSON response into v.
func fetchJSON(req *http.Request, v any) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

// redirectToApp sends the browser to the frontend's OAuth landing page with
// the outcome in the URL fragment.
func redirectToApp(w http.ResponseWriter, r *http.Request, fragment string) {
	http.Redirect(w, r, "/auth/callback#"+fragment, http.StatusFound)
}
