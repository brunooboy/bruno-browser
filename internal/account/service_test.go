package account

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestOAuthConfiguredRequiresOnlyPublicClientID(t *testing.T) {
	service, err := New(t.TempDir(), Config{ClientID: "1534690836594425896"})
	if err != nil {
		t.Fatal(err)
	}
	if !service.OAuthConfigured() {
		t.Fatal("public Discord client should not require a client secret")
	}
}

func TestRandomPKCEProducesS256Pair(t *testing.T) {
	verifier, challenge, err := randomPKCE()
	if err != nil {
		t.Fatal(err)
	}
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Fatalf("unexpected PKCE verifier length %d", len(verifier))
	}
	if verifier == challenge || len(challenge) != 43 {
		t.Fatalf("invalid PKCE challenge %q", challenge)
	}
}

func TestExchangeCodeUsesPKCEWithoutClientSecret(t *testing.T) {
	service, err := New(t.TempDir(), Config{ClientID: "public-client"})
	if err != nil {
		t.Fatal(err)
	}
	service.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != discordTokenURL {
			t.Fatalf("unexpected endpoint %q", request.URL)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if request.Form.Get("client_id") != "public-client" || request.Form.Get("code_verifier") != "verifier-value" {
			t.Fatalf("PKCE form is incomplete: %v", request.Form)
		}
		if request.Form.Get("client_secret") != "" {
			t.Fatal("public client exchange exposed a client secret")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"discord-access","token_type":"Bearer"}`)),
		}, nil
	})}

	token, err := service.exchangeCode(context.Background(), "authorization-code", "verifier-value")
	if err != nil {
		t.Fatal(err)
	}
	if token != "discord-access" {
		t.Fatalf("unexpected token %q", token)
	}
}
