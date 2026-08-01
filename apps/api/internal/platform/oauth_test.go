package platform

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"sealplatform/api/internal/storage"
)

func newOAuthTestApp(t *testing.T) (*httptest.Server, *http.Client, *Service) {
	t.Helper()
	root := t.TempDir()
	state, err := NewStateStore(filepath.Join(root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	objects, err := storage.NewLocal(filepath.Join(root, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(state, objects, slog.New(slog.NewTextHandler(io.Discard, nil)), 1<<20)
	mux := http.NewServeMux()
	service.Register(mux)
	server := httptest.NewServer(mux)
	service.Origin = server.URL
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	return server, client, service
}

func TestOAuthProvidersHideConfigurationValues(t *testing.T) {
	server, client, service := newOAuthTestApp(t)
	defer server.Close()
	service.QQOAuth = DefaultQQOAuthConfig("qq-id", "super-secret", "", server.URL)
	service.GitHubOAuth = DefaultGitHubOAuthConfig("github-id", "super-secret", "", server.URL)
	service.GoogleOAuth = DefaultGoogleOAuthConfig("google-id", "super-secret", "", server.URL)

	response, err := client.Get(server.URL + "/api/v1/auth/oauth/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]bool
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result["qq"] || result["wechat"] || !result["github"] || !result["google"] {
		t.Fatalf("unexpected provider availability: %#v", result)
	}
}

func TestOAuthRejectsMismatchedState(t *testing.T) {
	server, client, service := newOAuthTestApp(t)
	defer server.Close()
	service.QQOAuth = DefaultQQOAuthConfig("qq-id", "qq-secret", "", server.URL)

	if _, err := client.Get(server.URL + "/api/v1/auth/oauth/qq/start"); err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(server.URL + "/api/v1/auth/oauth/qq/callback?code=code&state=wrong")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if location := response.Header.Get("Location"); location != server.URL+"/?oauth=invalid_state" {
		t.Fatalf("unexpected redirect: %s", location)
	}
	if len(service.Store.ListUsers()) != 0 {
		t.Fatal("invalid state must not create a user")
	}
}

func TestOAuthLoginCreatesSession(t *testing.T) {
	for _, provider := range []string{"qq", "wechat", "github", "google"} {
		t.Run(provider, func(t *testing.T) {
			providerServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				if request.URL.Path == "/profile" {
					if request.Header.Get("Authorization") != "Bearer token" {
						t.Error("profile request must use a bearer token")
					}
					if provider == "github" {
						_, _ = writer.Write([]byte(`{"id":12345,"login":"seal-dev","name":"Seal Developer"}`))
					} else {
						_, _ = writer.Write([]byte(`{"sub":"google-subject","name":"Seal Google User"}`))
					}
					return
				}
				if provider == "qq" {
					if request.URL.Query().Get("client_secret") != "provider-secret" {
						t.Error("QQ secret missing from server-side token exchange")
					}
					_, _ = writer.Write([]byte(`{"access_token":"token","openid":"qq-openid"}`))
					return
				}
				if provider == "wechat" && request.URL.Query().Get("secret") != "provider-secret" {
					t.Error("WeChat secret missing from server-side token exchange")
				}
				if provider == "wechat" {
					_, _ = writer.Write([]byte(`{"access_token":"token","openid":"wechat-openid"}`))
					return
				}
				if err := request.ParseForm(); err != nil {
					t.Error(err)
				}
				if request.Method != http.MethodPost || request.Form.Get("client_secret") != "provider-secret" {
					t.Error("GitHub/Google token exchange must post the server-side secret")
				}
				if len(request.Form.Get("code_verifier")) < 43 {
					t.Error("GitHub/Google token exchange must include the PKCE verifier")
				}
				_, _ = writer.Write([]byte(`{"access_token":"token"}`))
			}))
			defer providerServer.Close()

			server, client, service := newOAuthTestApp(t)
			defer server.Close()
			config := OAuthProviderConfig{ClientID: "provider-id", ClientSecret: "provider-secret", RedirectURL: server.URL + "/api/v1/auth/oauth/" + provider + "/callback", AuthorizeURL: providerServer.URL + "/authorize", TokenURL: providerServer.URL + "/token", OpenIDURL: providerServer.URL + "/openid", ProfileURL: providerServer.URL + "/profile"}
			if provider == "qq" {
				service.QQOAuth = config
			} else {
				switch provider {
				case "wechat":
					service.WeChatOAuth = config
				case "github":
					service.GitHubOAuth = config
				case "google":
					service.GoogleOAuth = config
				}
			}

			start, err := client.Get(server.URL + "/api/v1/auth/oauth/" + provider + "/start")
			if err != nil {
				t.Fatal(err)
			}
			start.Body.Close()
			location, err := url.Parse(start.Header.Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			state := location.Query().Get("state")
			if state == "" || location.Query().Get("response_type") != "code" {
				t.Fatalf("invalid authorization redirect: %s", location)
			}
			if provider == "wechat" && location.Query().Get("scope") != "snsapi_login" {
				t.Fatal("WeChat website login must request snsapi_login")
			}
			if provider == "google" && location.Query().Get("scope") != "openid profile email" {
				t.Fatal("Google login must request OIDC identity scopes")
			}
			if (provider == "github" || provider == "google") && (location.Query().Get("code_challenge") == "" || location.Query().Get("code_challenge_method") != "S256") {
				t.Fatal("GitHub/Google authorization must use PKCE S256")
			}

			callback, err := client.Get(server.URL + "/api/v1/auth/oauth/" + provider + "/callback?code=valid-code&state=" + url.QueryEscape(state))
			if err != nil {
				t.Fatal(err)
			}
			callback.Body.Close()
			if callback.Header.Get("Location") != server.URL+"/?oauth=success" {
				t.Fatalf("unexpected callback redirect: %s", callback.Header.Get("Location"))
			}

			me, err := client.Get(server.URL + "/api/v1/auth/me")
			if err != nil {
				t.Fatal(err)
			}
			defer me.Body.Close()
			if me.StatusCode != http.StatusOK {
				t.Fatalf("OAuth callback did not create a session: %d", me.StatusCode)
			}
			var body struct {
				User userView `json:"user"`
			}
			if err := json.NewDecoder(me.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.User.AuthProvider != provider || body.User.DisplayName == "" {
				t.Fatalf("unexpected social user: %#v", body.User)
			}
		})
	}
}
