package handlers

import (
	"cmp"
	"context"
	"net/http"

	"golang.org/x/oauth2"
)

// NewYandex returns the Yandex OAuth provider.
func NewYandex(clientID, clientSecret, redirectURL string) *Provider {
	return &Provider{
		Name: "yandex",
		OAuth: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint: oauth2.Endpoint{
				AuthURL:   "https://oauth.yandex.ru/authorize",
				TokenURL:  "https://oauth.yandex.ru/token",
				AuthStyle: oauth2.AuthStyleInHeader,
			},
			// A stable ID and a name, nothing else: every extra permission
			// makes the consent screen scarier.
			Scopes: []string{"login:info"},
		},
		ProfileURL:   "https://login.yandex.ru/info?format=json",
		fetchProfile: yandexProfile,
	}
}

func yandexProfile(ctx context.Context, p *Provider, accessToken string) (oauthProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.ProfileURL, nil)
	if err != nil {
		return oauthProfile{}, err
	}
	req.Header.Set("Authorization", "OAuth "+accessToken)
	var info struct {
		ID          string `json:"id"`
		Login       string `json:"login"`
		DisplayName string `json:"display_name"`
		RealName    string `json:"real_name"`
	}
	if err := fetchJSON(req, &info); err != nil {
		return oauthProfile{}, err
	}
	return oauthProfile{subject: info.ID, name: cmp.Or(info.RealName, info.DisplayName, info.Login)}, nil
}
