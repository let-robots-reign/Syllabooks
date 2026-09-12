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
		echoParams:   []string{"device_id", "state"},
		fetchProfile: vkProfile,
	}
}

func vkProfile(ctx context.Context, p *Provider, accessToken string) (oauthProfile, error) {
	form := url.Values{"client_id": {p.OAuth.ClientID}, "access_token": {accessToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ProfileURL, strings.NewReader(form.Encode()))
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
