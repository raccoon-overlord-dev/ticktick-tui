package auth

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"

	"ttui/internal/config"
)

// Auth is the content of ~/.config/ttui/auth.toml.
type Auth struct {
	Method       string    `toml:"method"` // oauth | token
	Server       string    `toml:"server"`
	AccessToken  string    `toml:"access_token"`
	RefreshToken string    `toml:"refresh_token,omitempty"`
	ExpiresAt    time.Time `toml:"expires_at,omitzero"`
}

func Path() string { return filepath.Join(config.Dir(), "auth.toml") }

// Load returns nil, nil when the user is not signed in.
func Load() (*Auth, error) {
	var a Auth
	if _, err := toml.DecodeFile(Path(), &a); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if a.AccessToken == "" {
		return nil, nil
	}
	return &a, nil
}

// Save writes auth.toml atomically with mode 0600 in a 0700 directory.
func Save(a *Auth) error {
	dir := config.Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".auth-*.toml") // CreateTemp uses 0600
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := toml.NewEncoder(f).Encode(a); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), Path())
}

// FromToken converts an OAuth token response into stored auth.
func FromToken(t *Token, server string) *Auth {
	a := &Auth{Method: "oauth", Server: server, AccessToken: t.AccessToken, RefreshToken: t.RefreshToken}
	if t.ExpiresIn > 0 {
		a.ExpiresAt = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second).UTC().Truncate(time.Second)
	}
	return a
}

// SignOut removes auth.toml and the snapshot cache. Missing files are fine.
func SignOut() error {
	for _, p := range []string{Path(), filepath.Join(config.CacheDir(), "snapshot.json")} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
