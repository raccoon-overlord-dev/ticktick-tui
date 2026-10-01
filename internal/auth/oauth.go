// Package auth implements TickTick OAuth 2 (authorization code + PKCE) and token storage.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	AuthorizeURL = "https://ticktick.com/oauth/authorize"
	TokenURL     = "https://ticktick.com/oauth/token"
	Scopes       = "tasks:read tasks:write"
	RedirectURI  = "http://localhost:8421/callback"
	// ponytail: IPv4 only; browsers fall back from ::1 to 127.0.0.1 for "localhost".
	CallbackAddr = "127.0.0.1:8421"
	Timeout      = 3 * time.Minute
)

// Token is the OAuth token response. RefreshToken and ExpiresIn may be empty.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// Flow is one sign-in attempt. Create with Start, then call Wait.
type Flow struct {
	URL string // authorize URL to open in the browser

	clientID, tokenURL, redirectURI string
	state, verifier                 string
	srv                             *http.Server
	result                          chan flowResult
}

type flowResult struct {
	tok *Token
	err error
}

// ClientID is ttui's public OAuth client id (PKCE, no secret). TTUI_CLIENT_ID overrides it.
var ClientID = "FaUTXn48KU9vE33utJ"

// Start listens on CallbackAddr for the callback and builds the authorize URL.
func Start() (*Flow, error) {
	id := ClientID
	if v := os.Getenv("TTUI_CLIENT_ID"); v != "" {
		id = v
	}
	return start(id, CallbackAddr, RedirectURI, AuthorizeURL, TokenURL)
}

func start(clientID, addr, redirectURI, authorizeURL, tokenURL string) (*Flow, error) {
	if clientID == "" {
		return nil, errors.New("missing OAuth client id")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("callback port busy: %w", err)
	}
	verifier, challenge := newPKCE()
	f := &Flow{
		clientID: clientID, tokenURL: tokenURL, redirectURI: redirectURI,
		state: randString(16), verifier: verifier, result: make(chan flowResult, 1),
	}
	f.URL = authorizeURL + "?" + url.Values{
		"client_id":             {clientID},
		"scope":                 {Scopes},
		"state":                 {f.state},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", f.callback)
	f.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go f.srv.Serve(ln)
	return f, nil
}

// Wait blocks until the callback completes, ctx ends, or Timeout passes. It always stops the server.
func (f *Flow) Wait(ctx context.Context) (*Token, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	defer f.srv.Close()
	select {
	case r := <-f.result:
		return r.tok, r.err
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("timed out waiting for the browser; press o to retry")
		}
		return nil, ctx.Err()
	}
}

// Cancel stops the callback server without waiting.
func (f *Flow) Cancel() { f.srv.Close() }

func (f *Flow) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var res flowResult
	switch {
	case q.Get("state") != f.state:
		res.err = errors.New("OAuth state mismatch; try again")
	case q.Get("error") != "":
		res.err = fmt.Errorf("authorization denied: %s", q.Get("error"))
	case q.Get("code") == "":
		res.err = errors.New("no authorization code in callback")
	default:
		res.tok, res.err = f.exchange(r.Context(), q.Get("code"))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if res.err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, page("Sign-in failed. Return to your terminal for details."))
	} else {
		fmt.Fprint(w, page("Signed in. You can close this tab and return to your terminal."))
	}
	select {
	case f.result <- res:
	default: // a result is already pending; ignore duplicate callbacks
	}
}

// exchange trades the code for a token (PKCE, no client secret).
func (f *Flow) exchange(ctx context.Context, code string) (*Token, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {f.clientID},
		"code_verifier": {f.verifier},
		"grant_type":    {"authorization_code"},
		"scope":         {Scopes},
		"redirect_uri":  {f.redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("token exchange failed: %d %s", resp.StatusCode, body)
	}
	var tok Token
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return nil, fmt.Errorf("token exchange: unexpected response (%d bytes)", len(body))
	}
	return &tok, nil
}

func newPKCE() (verifier, challenge string) {
	verifier = randString(32)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func randString(n int) string {
	b := make([]byte, n)
	rand.Read(b) // never fails (crypto/rand, Go 1.24+)
	return base64.RawURLEncoding.EncodeToString(b)
}

func page(msg string) string {
	return `<!doctype html><meta charset="utf-8"><title>ttui</title>` +
		`<body style="font-family:monospace;background:#111115;color:#ecebf2;display:grid;place-items:center;height:100vh;margin:0">` +
		`<p>` + msg + `</p>`
}
