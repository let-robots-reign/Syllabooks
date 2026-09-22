package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

// NewVK returns the VK ID provider (id.vk.com, OAuth 2.1). The code exchange
// takes no client secret, because PKCE stands in for it. VK ID also wants the
// callback's device_id and state repeated in the token request.
func NewVK(clientID, redirectURL string) *Provider {
	return &Provider{
		Name: "vk",
		OAuth: oauth2.Config{
			ClientID:    clientID,
			RedirectURL: redirectURL,
			Endpoint: oauth2.Endpoint{
				AuthURL:   "https://id.vk.com/authorize",
				TokenURL:  "https://id.vk.com/oauth2/auth",
				AuthStyle: oauth2.AuthStyleInParams,
			},
			// VK ID's narrowest scope. It also covers photo, sex and
			// birthday; only the ID and the name are kept.
			Scopes: []string{"vkid.personal_info"},
		},
		ProfileURL:   "https://id.vk.com/oauth2/user_info",
		pkceMethod:   "s256",
		exchangeCode: vkExchangeCode,
		fetchProfile: vkProfile,
	}
}

// vkExchangeCode follows VK ID's wire format. Unlike a conventional OAuth
// token endpoint, VK requires every exchange parameter except code in the URL
// query. golang.org/x/oauth2 puts them all in the POST body instead.
func vkExchangeCode(ctx context.Context, p *Provider, callback url.Values, verifier string) (string, error) {
	code, state, deviceID := callback.Get("code"), callback.Get("state"), callback.Get("device_id")
	if code == "" || state == "" || deviceID == "" || verifier == "" {
		return "", fmt.Errorf("incomplete VK callback")
	}

	tokenURL, err := url.Parse(p.OAuth.Endpoint.TokenURL)
	if err != nil {
		return "", err
	}
	query := tokenURL.Query()
	query.Set("grant_type", "authorization_code")
	query.Set("redirect_uri", p.OAuth.RedirectURL)
	query.Set("client_id", p.OAuth.ClientID)
	query.Set("code_verifier", verifier)
	query.Set("state", state)
	query.Set("device_id", deviceID)
	tokenURL.RawQuery = query.Encode()

	form := url.Values{"code": {code}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var result struct {
		AccessToken      string `json:"access_token"`
		State            string `json:"state"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := fetchJSON(req, &result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("%s: %s", result.Error, result.ErrorDescription)
	}
	if result.State != state {
		return "", fmt.Errorf("token response state does not match callback")
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("token response has no access token")
	}
	return result.AccessToken, nil
}

func vkProfile(ctx context.Context, p *Provider, accessToken string) (oauthProfile, error) {
	profileURL, err := url.Parse(p.ProfileURL)
	if err != nil {
		return oauthProfile{}, err
	}
	query := profileURL.Query()
	query.Set("client_id", p.OAuth.ClientID)
	profileURL.RawQuery = query.Encode()

	form := url.Values{"access_token": {accessToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, profileURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return oauthProfile{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var info struct {
		User struct {
			// json.Number takes the ID as a JSON string or a number.
			UserID    json.Number `json:"user_id"`
			FirstName string      `json:"first_name"`
			LastName  string      `json:"last_name"`
		} `json:"user"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := fetchJSON(req, &info); err != nil {
		return oauthProfile{}, err
	}
	if info.Error != "" {
		return oauthProfile{}, fmt.Errorf("%s: %s", info.Error, info.ErrorDescription)
	}
	return oauthProfile{
		subject: info.User.UserID.String(),
		name:    strings.TrimSpace(info.User.FirstName + " " + info.User.LastName),
	}, nil
}
