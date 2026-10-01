package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"ttui/internal/config"
)

func TestPKCE(t *testing.T) {
	v, c := newPKCE()
	sum := sha256.Sum256([]byte(v))
	if len(v) < 43 || c != base64.RawURLEncoding.EncodeToString(sum[:]) {
		t.Fatalf("bad pkce: %q %q", v, c)
	}
}

func TestFlow(t *testing.T) {
	// Fake token endpoint: accepts only "good-code" with a PKCE verifier.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code") != "good-code" || r.Form.Get("code_verifier") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"access_token":"fake-access","token_type":"bearer","expires_in":3600}`))
	}))
	defer tokenSrv.Close()

	run := func(code, stateOverride string) (*Token, error) {
		f, err := start("cid", "127.0.0.1:0", "http://x/callback", "https://auth.example/authorize", tokenSrv.URL)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(f.URL)
		state := u.Query().Get("state")
		if stateOverride != "" {
			state = stateOverride
		}
		rec := httptest.NewRecorder()
		f.callback(rec, httptest.NewRequest("GET", "/callback?code="+code+"&state="+state, nil))
		return f.Wait(context.Background())
	}

	tok, err := run("good-code", "")
	if err != nil || tok.AccessToken != "fake-access" || tok.ExpiresIn != 3600 {
		t.Fatalf("flow: %+v %v", tok, err)
	}
	if _, err := run("good-code", "wrong-state"); err == nil {
		t.Fatal("state mismatch accepted")
	}
	if _, err := run("bad-code", ""); err == nil {
		t.Fatal("bad code accepted")
	}
}

func TestStore(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	if a, err := Load(); a != nil || err != nil {
		t.Fatalf("empty Load = %v, %v", a, err)
	}
	if err := Save(FromToken(&Token{AccessToken: "fake", ExpiresIn: 60}, "ticktick.com")); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(Path())
	dst, _ := os.Stat(config.Dir())
	if st.Mode().Perm() != 0o600 || dst.Mode().Perm() != 0o700 {
		t.Fatalf("perms file %v dir %v", st.Mode().Perm(), dst.Mode().Perm())
	}
	a, err := Load()
	if err != nil || a.AccessToken != "fake" || a.Method != "oauth" || a.ExpiresAt.IsZero() {
		t.Fatalf("Load = %+v, %v", a, err)
	}
	if err := SignOut(); err != nil {
		t.Fatal(err)
	}
	if a, _ := Load(); a != nil {
		t.Fatal("still signed in after SignOut")
	}
}
