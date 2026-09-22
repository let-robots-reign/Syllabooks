package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestVKExchangeAndProfileRequestFormat(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for name, want := range map[string]string{
			"grant_type":    "authorization_code",
			"redirect_uri":  "https://example.test/callback",
			"client_id":     "client-id",
			"code_verifier": "verifier",
			"state":         "state",
			"device_id":     "device",
		} {
			if got := query.Get(name); got != want {
				t.Errorf("token query %s = %q, want %q", name, got, want)
			}
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.PostForm.Get("code"); got != "code" {
			t.Errorf("token body code = %q, want code", got)
		}
		if len(r.PostForm) != 1 {
			t.Errorf("token body = %v, want only code", r.PostForm)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"access-token","state":"state"}`)
	})
	mux.HandleFunc("POST /profile", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("client_id"); got != "client-id" {
			t.Errorf("profile query client_id = %q, want client-id", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.PostForm.Get("access_token"); got != "access-token" {
			t.Errorf("profile body access_token = %q, want access-token", got)
		}
		if len(r.PostForm) != 1 {
			t.Errorf("profile body = %v, want only access_token", r.PostForm)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"user":{"user_id":123,"first_name":"Иван","last_name":"Петров"}}`)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	provider := NewVK("client-id", "https://example.test/callback")
	provider.OAuth.Endpoint.TokenURL = server.URL + "/token"
	provider.ProfileURL = server.URL + "/profile"

	accessToken, err := vkExchangeCode(context.Background(), provider, url.Values{
		"code":      {"code"},
		"state":     {"state"},
		"device_id": {"device"},
	}, "verifier")
	if err != nil {
		t.Fatalf("exchange code: %v", err)
	}
	profile, err := vkProfile(context.Background(), provider, accessToken)
	if err != nil {
		t.Fatalf("fetch profile: %v", err)
	}
	if profile.subject != "123" || profile.name != "Иван Петров" {
		t.Errorf("profile = %+v", profile)
	}
}

func TestVKAuthorizationUsesProviderPKCESpelling(t *testing.T) {
	t.Parallel()

	provider := NewVK("client-id", "https://example.test/callback")
	server := httptest.NewServer((&Server{VK: provider}).Routes())
	t.Cleanup(server.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	response, err := client.Get(server.URL + "/api/auth/vk")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if got := location.Query().Get("code_challenge_method"); got != "s256" {
		t.Errorf("code_challenge_method = %q, want s256", got)
	}
}

func TestVKExchangeRejectsMismatchedState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"access-token","state":"other"}`)
	}))
	t.Cleanup(server.Close)
	provider := NewVK("client-id", "https://example.test/callback")
	provider.OAuth.Endpoint.TokenURL = server.URL

	_, err := vkExchangeCode(context.Background(), provider, url.Values{
		"code":      {"code"},
		"state":     {"state"},
		"device_id": {"device"},
	}, "verifier")
	if err == nil {
		t.Fatal("exchange code succeeded with a mismatched state")
	}
}
