package account

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bruno-browser/internal/storage"
)

const (
	accountFileName     = "account.json"
	callbackAddress     = "localhost:34115"
	callbackURL         = "http://localhost:34115/callback"
	discordAuthorizeURL = "https://discord.com/oauth2/authorize"
	discordTokenURL     = "https://discord.com/api/v10/oauth2/token"
	discordUserURL      = "https://discord.com/api/v10/users/@me"
)

var ErrOAuthNotConfigured = errors.New("Discord OAuth is not configured")

type Config struct {
	ClientID     string
	ClientSecret string
	AdminID      string
}

type User struct {
	ID         string    `json:"id"`
	Username   string    `json:"username"`
	GlobalName string    `json:"globalName,omitempty"`
	Avatar     string    `json:"avatar,omitempty"`
	AvatarURL  string    `json:"avatarUrl,omitempty"`
	LoggedInAt time.Time `json:"loggedInAt"`
	IsAdmin    bool      `json:"isAdmin"`
}

type discordUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
	Avatar     string `json:"avatar"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

type Service struct {
	path    string
	config  Config
	client  *http.Client
	mu      sync.Mutex
	loginMu sync.Mutex
}

func New(dataRoot string, config Config) (*Service, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return nil, errors.New("data root is required")
	}
	config.ClientID = strings.TrimSpace(config.ClientID)
	config.ClientSecret = strings.TrimSpace(config.ClientSecret)
	config.AdminID = strings.TrimSpace(config.AdminID)
	return &Service{
		path:   filepath.Join(dataRoot, accountFileName),
		config: config,
		client: &http.Client{Timeout: 20 * time.Second},
	}, nil
}

func (s *Service) OAuthConfigured() bool {
	return s.config.ClientID != "" && s.config.ClientSecret != ""
}

func (s *Service) Get(ctx context.Context) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open Discord account: %w", err)
	}
	defer file.Close()
	var user User
	decoder := json.NewDecoder(io.LimitReader(file, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&user); err != nil {
		return nil, fmt.Errorf("decode Discord account: %w", err)
	}
	user.IsAdmin = s.config.AdminID != "" && user.ID == s.config.AdminID
	return &user, nil
}

func (s *Service) Login(ctx context.Context, openBrowser func(string) error) (User, error) {
	if !s.OAuthConfigured() {
		return User{}, ErrOAuthNotConfigured
	}
	if openBrowser == nil {
		return User{}, errors.New("browser opener is required")
	}
	s.loginMu.Lock()
	defer s.loginMu.Unlock()

	listener, err := net.Listen("tcp", callbackAddress)
	if err != nil {
		return User{}, fmt.Errorf("start Discord callback on %s: %w", callbackAddress, err)
	}
	state, err := randomState()
	if err != nil {
		_ = listener.Close()
		return User{}, err
	}
	type result struct{ code, errText string }
	resultChannel := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("state") != state {
			http.Error(writer, "Estado OAuth inválido. Volte ao bruno browser.", http.StatusBadRequest)
			return
		}
		response := result{code: query.Get("code"), errText: query.Get("error")}
		select {
		case resultChannel <- response:
		default:
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(writer, callbackPage(response.code != "", response.errText))
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	}()

	authorizeURL := discordAuthorizeURL + "?" + url.Values{
		"client_id":     {s.config.ClientID},
		"redirect_uri":  {callbackURL},
		"response_type": {"code"},
		"scope":         {"identify"},
		"state":         {state},
		"prompt":        {"consent"},
	}.Encode()
	if err := openBrowser(authorizeURL); err != nil {
		return User{}, fmt.Errorf("open Discord authorization: %w", err)
	}
	waitContext, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	var callback result
	select {
	case callback = <-resultChannel:
	case <-waitContext.Done():
		return User{}, fmt.Errorf("Discord login timed out: %w", waitContext.Err())
	}
	if callback.errText != "" {
		return User{}, fmt.Errorf("Discord denied authorization: %s", callback.errText)
	}
	if callback.code == "" {
		return User{}, errors.New("Discord callback did not contain an authorization code")
	}
	token, err := s.exchangeCode(waitContext, callback.code)
	if err != nil {
		return User{}, err
	}
	user, err := s.fetchUser(waitContext, token)
	if err != nil {
		return User{}, err
	}
	user.LoggedInAt = time.Now().UTC()
	user.IsAdmin = s.config.AdminID != "" && user.ID == s.config.AdminID
	s.mu.Lock()
	err = storage.WriteJSONAtomic(s.path, user, 0o600)
	s.mu.Unlock()
	if err != nil {
		return User{}, fmt.Errorf("save Discord account: %w", err)
	}
	return user, nil
}

func (s *Service) Logout(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove Discord account: %w", err)
	}
	return nil
}

func (s *Service) exchangeCode(ctx context.Context, code string) (string, error) {
	values := url.Values{
		"client_id":     {s.config.ClientID},
		"client_secret": {s.config.ClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {callbackURL},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, discordTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("exchange Discord authorization code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Discord token endpoint returned status %d", response.StatusCode)
	}
	var token tokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return "", fmt.Errorf("decode Discord token: %w", err)
	}
	if token.AccessToken == "" {
		return "", errors.New("Discord returned an empty access token")
	}
	return token.AccessToken, nil
}

func (s *Service) fetchUser(ctx context.Context, accessToken string) (User, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, discordUserURL, nil)
	if err != nil {
		return User{}, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := s.client.Do(request)
	if err != nil {
		return User{}, fmt.Errorf("load Discord user: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return User{}, fmt.Errorf("Discord user endpoint returned status %d", response.StatusCode)
	}
	var remote discordUser
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&remote); err != nil {
		return User{}, fmt.Errorf("decode Discord user: %w", err)
	}
	if remote.ID == "" || remote.Username == "" {
		return User{}, errors.New("Discord user response is incomplete")
	}
	avatarURL := ""
	if remote.Avatar != "" {
		remoteAvatarURL := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png?size=128", url.PathEscape(remote.ID), url.PathEscape(remote.Avatar))
		avatarURL = s.downloadAvatarDataURL(ctx, remoteAvatarURL)
	}
	return User{ID: remote.ID, Username: remote.Username, GlobalName: remote.GlobalName, Avatar: remote.Avatar, AvatarURL: avatarURL}, nil
}

func (s *Service) downloadAvatarDataURL(ctx context.Context, rawURL string) string {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ""
	}
	response, err := s.client.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	contentType := response.Header.Get("Content-Type")
	if contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp" {
		return ""
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(payload) == 0 || len(payload) > 2<<20 {
		return ""
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(payload)
}

func randomState() (string, error) {
	payload := make([]byte, 32)
	if _, err := rand.Read(payload); err != nil {
		return "", fmt.Errorf("create OAuth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func callbackPage(success bool, message string) string {
	title := "Login concluído"
	text := "Você pode fechar esta janela e voltar ao bruno browser."
	color := "#42ff91"
	if !success {
		title = "Login não concluído"
		text = "Discord retornou: " + html.EscapeString(message)
		color = "#ff5f6d"
	}
	return `<!doctype html><html lang="pt-BR"><meta charset="utf-8"><title>` + title + `</title><style>body{margin:0;display:grid;place-items:center;min-height:100vh;background:#05080a;color:#dfe9e5;font-family:system-ui}.box{padding:32px;border:1px solid #25343a;background:#0a1013}h1{color:` + color + `;font-family:monospace}</style><div class="box"><h1>` + title + `</h1><p>` + text + `</p></div></html>`
}
