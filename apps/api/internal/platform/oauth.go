package platform

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const oauthStateCookie = "seal_oauth_state"
const oauthPKCECookie = "seal_oauth_pkce"

type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AuthorizeURL string
	TokenURL     string
	OpenIDURL    string
	ProfileURL   string
}

func oauthRedirect(explicit, origin, provider string) string {
	if explicit != "" {
		return explicit
	}
	return strings.TrimRight(origin, "/") + "/api/v1/auth/oauth/" + provider + "/callback"
}

func DefaultQQOAuthConfig(id, secret, redirect, origin string) OAuthProviderConfig {
	return OAuthProviderConfig{ClientID: id, ClientSecret: secret, RedirectURL: oauthRedirect(redirect, origin, "qq"), AuthorizeURL: "https://graph.qq.com/oauth2.0/authorize", TokenURL: "https://graph.qq.com/oauth2.0/token", OpenIDURL: "https://graph.qq.com/oauth2.0/me"}
}

func DefaultWeChatOAuthConfig(id, secret, redirect, origin string) OAuthProviderConfig {
	return OAuthProviderConfig{ClientID: id, ClientSecret: secret, RedirectURL: oauthRedirect(redirect, origin, "wechat"), AuthorizeURL: "https://open.weixin.qq.com/connect/qrconnect", TokenURL: "https://api.weixin.qq.com/sns/oauth2/access_token"}
}

func DefaultGitHubOAuthConfig(id, secret, redirect, origin string) OAuthProviderConfig {
	return OAuthProviderConfig{ClientID: id, ClientSecret: secret, RedirectURL: oauthRedirect(redirect, origin, "github"), AuthorizeURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", ProfileURL: "https://api.github.com/user"}
}

func DefaultGoogleOAuthConfig(id, secret, redirect, origin string) OAuthProviderConfig {
	return OAuthProviderConfig{ClientID: id, ClientSecret: secret, RedirectURL: oauthRedirect(redirect, origin, "google"), AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token", ProfileURL: "https://openidconnect.googleapis.com/v1/userinfo"}
}

func (config OAuthProviderConfig) Enabled() bool {
	return config.ClientID != "" && config.ClientSecret != "" && config.RedirectURL != ""
}

func (service *Service) oauthConfig(provider string) (OAuthProviderConfig, bool) {
	switch provider {
	case "qq":
		return service.QQOAuth, service.QQOAuth.Enabled()
	case "wechat":
		return service.WeChatOAuth, service.WeChatOAuth.Enabled()
	case "github":
		return service.GitHubOAuth, service.GitHubOAuth.Enabled()
	case "google":
		return service.GoogleOAuth, service.GoogleOAuth.Enabled()
	default:
		return OAuthProviderConfig{}, false
	}
}

func (service *Service) oauthProviders(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]bool{"qq": service.QQOAuth.Enabled(), "wechat": service.WeChatOAuth.Enabled(), "github": service.GitHubOAuth.Enabled(), "google": service.GoogleOAuth.Enabled()})
}

func (service *Service) oauthStart(writer http.ResponseWriter, request *http.Request) {
	provider := request.PathValue("provider")
	config, enabled := service.oauthConfig(provider)
	if !enabled {
		writeError(writer, http.StatusNotFound, "OAUTH_UNAVAILABLE", "该快捷登录尚未配置")
		return
	}
	state := randomID("oauth_")
	http.SetCookie(writer, &http.Cookie{Name: oauthStateCookie, Value: provider + "." + state, Path: "/api/v1/auth/oauth/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: request.TLS != nil, MaxAge: 600})
	parameters := url.Values{"response_type": {"code"}, "client_id": {config.ClientID}, "redirect_uri": {config.RedirectURL}, "state": {state}}
	if provider == "github" || provider == "google" {
		verifier := randomID("") + randomID("")
		challenge := sha256.Sum256([]byte(verifier))
		parameters.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
		parameters.Set("code_challenge_method", "S256")
		http.SetCookie(writer, &http.Cookie{Name: oauthPKCECookie, Value: provider + "." + verifier, Path: "/api/v1/auth/oauth/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: request.TLS != nil, MaxAge: 600})
	}
	if provider == "wechat" {
		parameters.Set("appid", config.ClientID)
		parameters.Del("client_id")
		parameters.Set("scope", "snsapi_login")
		http.Redirect(writer, request, config.AuthorizeURL+"?"+parameters.Encode()+"#wechat_redirect", http.StatusFound)
		return
	}
	if provider == "qq" {
		parameters.Set("scope", "get_user_info")
	} else if provider == "google" {
		parameters.Set("scope", "openid profile email")
		parameters.Set("prompt", "select_account")
	}
	http.Redirect(writer, request, config.AuthorizeURL+"?"+parameters.Encode(), http.StatusFound)
}

func (service *Service) oauthCallback(writer http.ResponseWriter, request *http.Request) {
	provider := request.PathValue("provider")
	config, enabled := service.oauthConfig(provider)
	if !enabled {
		service.oauthReturn(writer, request, "unavailable")
		return
	}
	stateCookie, cookieErr := request.Cookie(oauthStateCookie)
	expected := provider + "." + request.URL.Query().Get("state")
	http.SetCookie(writer, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/api/v1/auth/oauth/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: request.TLS != nil, MaxAge: -1})
	pkceCookie, _ := request.Cookie(oauthPKCECookie)
	http.SetCookie(writer, &http.Cookie{Name: oauthPKCECookie, Value: "", Path: "/api/v1/auth/oauth/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: request.TLS != nil, MaxAge: -1})
	if cookieErr != nil || len(stateCookie.Value) != len(expected) || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(expected)) != 1 || request.URL.Query().Get("code") == "" {
		service.oauthReturn(writer, request, "invalid_state")
		return
	}
	verifier := ""
	if pkceCookie != nil && strings.HasPrefix(pkceCookie.Value, provider+".") {
		verifier = strings.TrimPrefix(pkceCookie.Value, provider+".")
	}
	if (provider == "github" || provider == "google") && len(verifier) < 43 {
		service.oauthReturn(writer, request, "invalid_state")
		return
	}
	identity, err := service.exchangeOAuth(request.Context(), provider, config, request.URL.Query().Get("code"), verifier)
	if err != nil {
		service.oauthReturn(writer, request, "provider_error")
		return
	}
	user, exists := service.Store.FindUserByExternalIdentity(provider, identity.Subject)
	if !exists {
		digest := sha256.Sum256([]byte(provider + ":" + identity.Subject))
		label := map[string]string{"qq": "QQ用户", "wechat": "微信用户", "github": "GitHub 用户", "google": "Google 用户"}[provider]
		if identity.DisplayName != "" {
			label = identity.DisplayName
		}
		user = User{ID: randomID("usr_"), Email: provider + "-" + hex.EncodeToString(digest[:12]) + "@oauth.local", MembershipLevel: "free", Status: "active", CreatedAt: time.Now().UTC(), Role: "user", AuthProvider: provider, AuthSubject: identity.Subject, DisplayName: label}
		if err := service.Store.SaveUser(user); err != nil {
			service.oauthReturn(writer, request, "store_error")
			return
		}
		service.audit(user.ID, "auth.oauth_register", user.ID, map[string]any{"provider": provider})
	}
	if user.Status != "active" || service.createSession(writer, request, user) != nil {
		service.oauthReturn(writer, request, "login_failed")
		return
	}
	service.audit(user.ID, "auth.oauth_login", user.ID, map[string]any{"provider": provider})
	service.oauthReturn(writer, request, "success")
}

func (service *Service) oauthReturn(writer http.ResponseWriter, request *http.Request, result string) {
	origin := strings.TrimRight(service.Origin, "/")
	if origin == "" {
		origin = "/"
	}
	target := origin + "/?oauth=" + url.QueryEscape(result)
	http.Redirect(writer, request, target, http.StatusFound)
}

type oauthIdentity struct {
	Subject     string
	DisplayName string
}

func (service *Service) exchangeOAuth(ctx context.Context, provider string, config OAuthProviderConfig, code, verifier string) (oauthIdentity, error) {
	client := service.OAuthHTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	parameters := url.Values{"code": {code}}
	if provider == "qq" || provider == "github" || provider == "google" {
		parameters.Set("grant_type", "authorization_code")
		parameters.Set("client_id", config.ClientID)
		parameters.Set("client_secret", config.ClientSecret)
		parameters.Set("redirect_uri", config.RedirectURL)
		if provider == "qq" {
			parameters.Set("fmt", "json")
			parameters.Set("need_openid", "1")
		}
		if verifier != "" {
			parameters.Set("code_verifier", verifier)
		}
	} else {
		parameters.Set("appid", config.ClientID)
		parameters.Set("secret", config.ClientSecret)
		parameters.Set("grant_type", "authorization_code")
	}
	tokenResponse := struct {
		AccessToken string `json:"access_token"`
		OpenID      string `json:"openid"`
		UnionID     string `json:"unionid"`
		Error       int    `json:"errcode"`
	}{}
	var tokenErr error
	if provider == "github" || provider == "google" {
		tokenErr = oauthPostFormJSON(ctx, client, config.TokenURL, parameters, &tokenResponse)
	} else {
		tokenErr = oauthGetJSON(ctx, client, config.TokenURL+"?"+parameters.Encode(), &tokenResponse)
	}
	if tokenErr != nil || tokenResponse.Error != 0 || tokenResponse.AccessToken == "" {
		return oauthIdentity{}, errors.New("OAuth token exchange failed")
	}
	if provider == "wechat" {
		if tokenResponse.OpenID != "" {
			return oauthIdentity{Subject: tokenResponse.OpenID}, nil
		}
		return oauthIdentity{}, errors.New("WeChat identity missing")
	}
	if provider == "github" {
		profile := struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Name  string `json:"name"`
		}{}
		if err := oauthBearerJSON(ctx, client, config.ProfileURL, tokenResponse.AccessToken, &profile); err != nil || profile.ID == 0 {
			return oauthIdentity{}, errors.New("GitHub identity lookup failed")
		}
		name := profile.Name
		if name == "" {
			name = profile.Login
		}
		return oauthIdentity{Subject: strconv.FormatInt(profile.ID, 10), DisplayName: name}, nil
	}
	if provider == "google" {
		profile := struct {
			Subject string `json:"sub"`
			Name    string `json:"name"`
		}{}
		if err := oauthBearerJSON(ctx, client, config.ProfileURL, tokenResponse.AccessToken, &profile); err != nil || profile.Subject == "" {
			return oauthIdentity{}, errors.New("Google identity lookup failed")
		}
		return oauthIdentity{Subject: profile.Subject, DisplayName: profile.Name}, nil
	}
	if tokenResponse.OpenID != "" {
		return oauthIdentity{Subject: tokenResponse.OpenID}, nil
	}
	openidResponse := struct {
		OpenID string `json:"openid"`
		Error  int    `json:"error"`
	}{}
	endpoint := config.OpenIDURL + "?" + url.Values{"access_token": {tokenResponse.AccessToken}, "fmt": {"json"}}.Encode()
	if err := oauthGetJSON(ctx, client, endpoint, &openidResponse); err != nil || openidResponse.Error != 0 || openidResponse.OpenID == "" {
		return oauthIdentity{}, errors.New("QQ OpenID lookup failed")
	}
	return oauthIdentity{Subject: openidResponse.OpenID}, nil
}

func oauthGetJSON(ctx context.Context, client *http.Client, endpoint string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("OAuth provider returned %d", response.StatusCode)
	}
	reader := io.LimitReader(response.Body, 1<<20)
	return json.NewDecoder(reader).Decode(target)
}

func oauthPostFormJSON(ctx context.Context, client *http.Client, endpoint string, form url.Values, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	return oauthDoJSON(client, request, target)
}

func oauthBearerJSON(ctx context.Context, client *http.Client, endpoint, token string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	return oauthDoJSON(client, request, target)
}

func oauthDoJSON(client *http.Client, request *http.Request, target any) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("OAuth provider returned %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target)
}
