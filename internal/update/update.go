// Package update checks GitHub for a newer ttui release and installs it over the running binary.
package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"ttui/internal/config"
)

// Repo is the project page, shown in Settings → About.
const Repo = "https://github.com/raccoon-overlord-dev/ticktick-tui"

// Releases is the GitHub releases page; TTUI_RELEASES overrides it (tests, forks).
var Releases = Repo + "/releases"

// Command is the manual fallback, the same as the README's install line.
const Command = "curl -fsSL https://raw.githubusercontent.com/raccoon-overlord-dev/ticktick-tui/main/install.sh | sh"

// ErrNotWritable means the binary's folder can't be written (e.g. root-owned): update with Command.
var ErrNotWritable = errors.New("can't write the ttui binary")

func init() {
	if r := os.Getenv("TTUI_RELEASES"); r != "" {
		Releases = r
	}
}

var client = &http.Client{
	Timeout:       time.Minute,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// stamp holds the last tag seen; its mtime is when it was checked.
func stamp() string { return filepath.Join(config.CacheDir(), "latest-version") }

// Check returns the latest release tag, asking GitHub at most once a day unless force is set
// (a manual check).
func Check(ctx context.Context, force bool) (string, error) {
	if fi, err := os.Stat(stamp()); err == nil && !force && time.Since(fi.ModTime()) < 24*time.Hour {
		b, err := os.ReadFile(stamp())
		return strings.TrimSpace(string(b)), err
	}
	tag, err := latest(ctx)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(config.CacheDir(), 0o700); err == nil {
		os.WriteFile(stamp(), []byte(tag+"\n"), 0o600)
	}
	return tag, nil
}

// latest reads the tag from the redirect of /releases/latest (no API rate limit involved).
func latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Releases+"/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/tag/") {
		return "", fmt.Errorf("no release found (HTTP %d)", resp.StatusCode)
	}
	return path.Base(loc), nil
}

// Newer reports whether tag is a later version than current ("v0.4.0" vs "0.3.0").
// A non-numeric current ("dev") is never updated.
func Newer(tag, current string) bool {
	parse := func(s string) []int {
		var v []int
		for _, p := range strings.Split(strings.TrimPrefix(s, "v"), ".") {
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil
			}
			v = append(v, n)
		}
		return v
	}
	t, c := parse(tag), parse(current)
	if t == nil || c == nil {
		return false
	}
	for i := range max(len(t), len(c)) {
		var x, y int
		if i < len(t) {
			x = t[i]
		}
		if i < len(c) {
			y = c[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// Apply downloads release tag, checks it against the release's checksums.txt and
// replaces the binary at exe. The running process keeps working; restart to use it.
func Apply(ctx context.Context, tag, exe string) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".ttui.new*")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotWritable, err)
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	defer tmp.Close()

	name := fmt.Sprintf("ttui_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	base := Releases + "/download/" + tag + "/"
	sums, err := get(ctx, base+"checksums.txt")
	if err != nil {
		return err
	}
	want := ""
	for _, l := range strings.Split(string(sums), "\n") {
		if sum, file, ok := strings.Cut(l, "  "); ok && file == name {
			want = sum
		}
	}
	if want == "" {
		return fmt.Errorf("%s is not in checksums.txt", name)
	}
	archive, err := get(ctx, base+name)
	if err != nil {
		return err
	}
	if got := sha256.Sum256(archive); hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s", name)
	}

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("no ttui binary in %s", name)
		}
		if err != nil {
			return err
		}
		if h.Name == "ttui" {
			break
		}
	}
	if _, err := io.Copy(tmp, tr); err != nil {
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), exe) // atomic, and safe while the old binary runs
}

func get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req) // follows the redirect to GitHub's file host
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", path.Base(url), resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
