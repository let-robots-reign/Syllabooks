package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"syllabooks/db/gen"
)

// The database tests drive the real handlers against DATABASE_URL. Each test
// creates its own users and deletes them afterwards; their sessions go with
// them (ON DELETE CASCADE).

func TestPasswordHash(t *testing.T) {
	hash := hashPassword("кошка1")
	for _, tc := range []struct {
		password string
		want     bool
	}{{"кошка1", true}, {"кошка2", false}, {"", false}} {
		got, err := checkPassword(hash, tc.password)
		if err != nil {
			t.Fatalf("check %q: %v", tc.password, err)
		}
		if got != tc.want {
			t.Errorf("check %q = %v, want %v", tc.password, got, tc.want)
		}
	}
	if hashPassword("кошка1") == hash {
		t.Error("two hashes of the same password are equal: the salt is not random")
	}
}

func TestCodeLogin(t *testing.T) {
	env := newTestEnv(t, Server{})
	_, code := env.newCodeUser(t)

	env.request(t, "POST", "/api/auth/code/check", "", map[string]string{"code": "NOPE00"}, http.StatusNotFound)

	// First visit: no password yet, so the student makes one up. The code is
	// typed in lowercase on purpose.
	check := env.request(t, "POST", "/api/auth/code/check", "", map[string]string{"code": strings.ToLower(code)}, http.StatusOK)
	if check["has_password"] != false {
		t.Fatalf("new user: has_password = %v, want false", check["has_password"])
	}
	// Five characters (ten bytes) is too short.
	env.request(t, "POST", "/api/auth/code/login", "", map[string]string{"code": code, "password": "кошка"}, http.StatusBadRequest)
	first := env.login(t, code, "кошка1")

	// Every visit after that: the same screen now asks for the password.
	check = env.request(t, "POST", "/api/auth/code/check", "", map[string]string{"code": code}, http.StatusOK)
	if check["has_password"] != true {
		t.Fatalf("after first login: has_password = %v, want true", check["has_password"])
	}
	env.request(t, "POST", "/api/auth/code/login", "", map[string]string{"code": code, "password": "кошка2"}, http.StatusUnauthorized)
	second := env.login(t, code, "кошка1")

	// Deleting a session row locks that client out at once, and only that one.
	env.request(t, "GET", "/api/me", first, nil, http.StatusOK)
	env.exec(t, "DELETE FROM sessions WHERE token_hash = $1", hashToken(first))
	env.request(t, "GET", "/api/me", first, nil, http.StatusUnauthorized)
	env.request(t, "GET", "/api/me", second, nil, http.StatusOK)

	env.request(t, "POST", "/api/auth/logout", second, nil, http.StatusNoContent)
	env.request(t, "GET", "/api/me", second, nil, http.StatusUnauthorized)
}

func TestBannedUserIsLockedOut(t *testing.T) {
	env := newTestEnv(t, Server{})
	id, code := env.newCodeUser(t)
	token := env.login(t, code, "кошка1")

	env.exec(t, "UPDATE users SET status = 'banned' WHERE id = $1", id)
	env.request(t, "GET", "/api/me", token, nil, http.StatusUnauthorized)
	env.request(t, "POST", "/api/auth/code/login", "", map[string]string{"code": code, "password": "кошка1"}, http.StatusForbidden)
}

func TestResetPassword(t *testing.T) {
	env := newTestEnv(t, Server{})
	adminID, adminCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET is_admin = true WHERE id = $1", adminID)
	admin := env.login(t, adminCode, "учитель")
	studentID, studentCode := env.newCodeUser(t)
	student := env.login(t, studentCode, "кошка1")
	path := "/api/admin/users/" + studentID.String() + "/reset-password"

	// To anyone but an admin, the route does not exist.
	env.request(t, "POST", path, "", nil, http.StatusNotFound)
	env.request(t, "POST", path, student, nil, http.StatusNotFound)

	env.request(t, "POST", path, admin, nil, http.StatusNoContent)
	env.request(t, "GET", "/api/me", student, nil, http.StatusUnauthorized)
	check := env.request(t, "POST", "/api/auth/code/check", "", map[string]string{"code": studentCode}, http.StatusOK)
	if check["has_password"] != false {
		t.Fatalf("after reset: has_password = %v, want false", check["has_password"])
	}
	env.login(t, studentCode, "собака1")

	env.request(t, "POST", "/api/admin/users/"+uuid.NewString()+"/reset-password", admin, nil, http.StatusNotFound)
}

func TestOAuthLogin(t *testing.T) {
	yandex, vk := fakeProviders(t)
	env := newTestEnv(t, Server{Yandex: yandex, VK: vk})

	for _, p := range []*Provider{yandex, vk} {
		t.Run(p.Name, func(t *testing.T) {
			// First login creates the user with the name from the provider.
			me := env.request(t, "GET", "/api/me", env.oauthToken(t, p), nil, http.StatusOK)
			env.deleteUserAfter(t, me["id"])
			if me["display_name"] != "Иван Петров" {
				t.Fatalf("display_name = %v, want Иван Петров", me["display_name"])
			}

			// The next login finds the same user, and the teacher's correction
			// of the name survives it.
			env.exec(t, "UPDATE users SET display_name = 'Ваня П.' WHERE id = $1", me["id"])
			again := env.request(t, "GET", "/api/me", env.oauthToken(t, p), nil, http.StatusOK)
			if again["id"] != me["id"] || again["display_name"] != "Ваня П." {
				t.Fatalf("second login: got %v, want user %v still named Ваня П.", again, me["id"])
			}

			for _, tc := range []struct {
				name   string
				tamper func(url.Values)
				want   string
			}{
				{"forged state", func(q url.Values) { q.Set("state", "forged") }, "/auth/callback#error=failed"},
				{"forged code", func(q url.Values) { q.Set("code", "forged") }, "/auth/callback#error=failed"},
				{"cancelled", func(q url.Values) { q.Del("code"); q.Set("error", "access_denied") }, "/auth/callback#error=cancelled"},
			} {
				if got := env.oauthLogin(t, p, tc.tamper); got != tc.want {
					t.Errorf("%s: redirected to %q, want %q", tc.name, got, tc.want)
				}
			}

			env.exec(t, "UPDATE users SET status = 'banned' WHERE id = $1", me["id"])
			if got := env.oauthLogin(t, p, nil); got != "/auth/callback#error=banned" {
				t.Errorf("banned user: redirected to %q", got)
			}
		})
	}
}

// fakeProviders serves Yandex and VK ID look-alikes that check what the real
// ones would: client credentials, PKCE, and the parameters VK wants echoed.
func fakeProviders(t *testing.T) (yandex, vk *Provider) {
	yandexID := rand.Text()
	vkID := strconv.FormatInt(time.Now().UnixNano(), 10)

	mux := http.NewServeMux()
	// The consent screen approves at once. The code it issues is the PKCE
	// challenge, so the token endpoint can check the verifier statelessly.
	mux.HandleFunc("GET /{provider}/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("client_id") != "client" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
			http.Error(w, "bad authorization request", http.StatusBadRequest)
			return
		}
		back := url.Values{"code": {q.Get("code_challenge")}, "state": {q.Get("state")}}
		if r.PathValue("provider") == "vk" {
			back.Set("device_id", "device-1")
		}
		http.Redirect(w, r, q.Get("redirect_uri")+"?"+back.Encode(), http.StatusFound)
	})
	mux.HandleFunc("POST /{provider}/token", func(w http.ResponseWriter, r *http.Request) {
		sum := sha256.Sum256([]byte(r.FormValue("code_verifier")))
		ok := base64.RawURLEncoding.EncodeToString(sum[:]) == r.FormValue("code")
		switch r.PathValue("provider") {
		case "yandex":
			id, secret, _ := r.BasicAuth()
			ok = ok && id == "client" && secret == "secret"
		case "vk":
			ok = ok && r.FormValue("client_id") == "client" &&
				r.FormValue("device_id") == "device-1" && r.FormValue("state") != ""
		}
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": "invalid_grant"}`)
			return
		}
		fmt.Fprint(w, `{"access_token": "access-token", "token_type": "Bearer", "expires_in": 3600}`)
	})
	mux.HandleFunc("GET /yandex/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "OAuth access-token" {
			http.Error(w, "", http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, `{"id": %q, "login": "ivan", "display_name": "ivan", "real_name": "Иван Петров"}`, yandexID)
	})
	mux.HandleFunc("POST /vk/user_info", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("client_id") != "client" || r.FormValue("access_token") != "access-token" {
			fmt.Fprint(w, `{"error": "invalid_token", "error_description": "bad token"}`)
			return
		}
		// A bare number: the stricter of the two forms the ID might take.
		fmt.Fprintf(w, `{"user": {"user_id": %s, "first_name": "Иван", "last_name": "Петров"}}`, vkID)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	yandex = NewYandex("client", "secret", "http://localhost/api/auth/callback/yandex")
	yandex.OAuth.Endpoint.AuthURL = srv.URL + "/yandex/authorize"
	yandex.OAuth.Endpoint.TokenURL = srv.URL + "/yandex/token"
	yandex.ProfileURL = srv.URL + "/yandex/info"
	vk = NewVK("client", "http://localhost/api/auth/callback/vk")
	vk.OAuth.Endpoint.AuthURL = srv.URL + "/vk/authorize"
	vk.OAuth.Endpoint.TokenURL = srv.URL + "/vk/token"
	vk.ProfileURL = srv.URL + "/vk/user_info"
	return yandex, vk
}

type testEnv struct {
	pool   *pgxpool.Pool
	srv    *httptest.Server
	client *http.Client
}

// newTestEnv serves srv's routes, with Pool filled in, over httptest.
func newTestEnv(t *testing.T, srv Server) *testEnv {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	srv.Pool = pool
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return &testEnv{pool: pool, srv: ts, client: client}
}

// newCodeUser creates a code user with no password. Test codes start with 0,
// which real codes never contain.
func (e *testEnv) newCodeUser(t *testing.T) (uuid.UUID, string) {
	t.Helper()
	code := "0" + rand.Text()[:5]
	user, err := gen.New(e.pool).CreateCodeUser(context.Background(),
		gen.CreateCodeUserParams{DisplayName: "Test " + code, Code: code})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	e.deleteUserAfter(t, user.ID)
	return user.ID, code
}

func (e *testEnv) deleteUserAfter(t *testing.T, id any) {
	t.Cleanup(func() {
		if _, err := e.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id); err != nil {
			t.Errorf("delete test user: %v", err)
		}
	})
}

func (e *testEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (e *testEnv) login(t *testing.T, code, password string) string {
	t.Helper()
	data := e.request(t, "POST", "/api/auth/code/login", "",
		map[string]string{"code": code, "password": password}, http.StatusOK)
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatalf("login returned no token: %v", data)
	}
	return token
}

// request sends a JSON request, checks the status and returns the decoded
// JSON body, or nil when the response isn't JSON.
func (e *testEnv) request(t *testing.T, method, path, token string, body any, wantStatus int) map[string]any {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, data := e.do(t, req)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: got %d %v, want %d", method, path, resp.StatusCode, data, wantStatus)
	}
	return data
}

func (e *testEnv) do(t *testing.T, req *http.Request) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	var data map[string]any
	if resp.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			t.Fatalf("%s %s: decode response: %v", req.Method, req.URL.Path, err)
		}
	}
	return resp, data
}

// redirect GETs rawURL with cookies and returns the response, which must be
// a redirect.
func (e *testEnv) redirect(t *testing.T, rawURL string, cookies []*http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, data := e.do(t, req)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("GET %s: got %d %v, want 302", req.URL.Path, resp.StatusCode, data)
	}
	return resp
}

// oauthLogin plays the browser through a login with p: start it, pass the
// fake consent screen, and come back to the callback, letting tamper edit the
// callback's query first. It returns where the callback redirected to.
func (e *testEnv) oauthLogin(t *testing.T, p *Provider, tamper func(url.Values)) string {
	t.Helper()
	start := e.redirect(t, e.srv.URL+"/api/auth/"+p.Name, nil)
	consent := e.redirect(t, start.Header.Get("Location"), nil)
	back, err := url.Parse(consent.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect back from the provider: %v", err)
	}
	query := back.Query()
	if tamper != nil {
		tamper(query)
	}
	callbackPath := "/api/auth/callback/" + p.Name
	callback := e.redirect(t, e.srv.URL+callbackPath+"?"+query.Encode(), start.Cookies())
	return callback.Header.Get("Location")
}

func (e *testEnv) oauthToken(t *testing.T, p *Provider) string {
	t.Helper()
	location := e.oauthLogin(t, p, nil)
	token, ok := strings.CutPrefix(location, "/auth/callback#token=")
	if !ok {
		t.Fatalf("callback redirected to %q, want a token", location)
	}
	return token
}
