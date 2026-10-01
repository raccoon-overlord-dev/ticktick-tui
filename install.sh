#!/bin/sh
# ttui installer (macOS and Linux; on Windows use WSL).
#
#   curl -fsSL https://raw.githubusercontent.com/raccoon-overlord-dev/ticktick-tui/main/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- --uninstall
#
# Environment:
#   TTUI_INSTALL_DIR   where to put the binary (default ~/.local/bin)
#   TTUI_FROM_SOURCE   path to a local checkout: build it with Go instead of downloading
#   TTUI_REPO          GitHub owner/repo to download from
#   TTUI_RELEASE_URL   base URL of the release files (default: the repo's latest GitHub release)
set -eu

REPO="${TTUI_REPO:-raccoon-overlord-dev/ticktick-tui}"
DIR="${TTUI_INSTALL_DIR:-$HOME/.local/bin}"
BASE="${TTUI_RELEASE_URL:-https://github.com/$REPO/releases/latest/download}"

say() { printf '%s\n' "$*"; }
die() { printf 'ttui install: %s\n' "$*" >&2; exit 1; }

fetch() { # url dest
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$1" -O "$2"
	else
		die "need curl or wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d' ' -f1
	else
		die "need sha256sum or shasum"
	fi
}

uninstall() {
	rm -f "$DIR/ttui"
	say "removed $DIR/ttui"
	cfg="${XDG_CONFIG_HOME:-$HOME/.config}/ttui"
	[ -d "$cfg" ] || exit 0
	printf 'Also remove %s (settings and sign-in)? [y/N] ' "$cfg"
	ans=""
	{ read -r ans </dev/tty; } 2>/dev/null || true # stdin is the script when piped; no tty = keep
	case "$ans" in
	y | Y | yes)
		rm -rf "$cfg" "${XDG_CACHE_HOME:-$HOME/.cache}/ttui"
		say "removed $cfg"
		;;
	*) say "kept $cfg" ;;
	esac
	exit 0
}

if [ $# -gt 0 ]; then
	[ "$1" = "--uninstall" ] || die "unknown argument: $1 (only --uninstall)"
	uninstall
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

if [ -n "${TTUI_FROM_SOURCE:-}" ]; then
	src=$TTUI_FROM_SOURCE
	command -v go >/dev/null 2>&1 || die "TTUI_FROM_SOURCE needs Go: https://go.dev/dl"
	[ -f "$src/go.mod" ] || die "$src is not a ttui checkout"
	ver=$(git -C "$src" describe --tags --always --dirty 2>/dev/null || echo dev)
	say "building ttui $ver from $src"
	(cd "$src" && CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$ver" -o "$tmp/ttui" ./cmd/ttui)
else
	case $(uname -s) in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) die "unsupported OS $(uname -s): ttui runs on Linux and macOS (use WSL on Windows)" ;;
	esac
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "unsupported CPU $(uname -m)" ;;
	esac
	name="ttui_${os}_${arch}.tar.gz"
	say "downloading $name"
	fetch "$BASE/$name" "$tmp/$name" || die "download failed: $BASE/$name"
	fetch "$BASE/checksums.txt" "$tmp/checksums.txt" || die "download failed: $BASE/checksums.txt"
	want=$(grep " $name\$" "$tmp/checksums.txt" | cut -d' ' -f1)
	[ -n "$want" ] || die "$name is not listed in checksums.txt"
	[ "$(sha256 "$tmp/$name")" = "$want" ] || die "checksum mismatch for $name: download corrupted or tampered with"
	tar -xzf "$tmp/$name" -C "$tmp" ttui
fi

mkdir -p "$DIR"
cp "$tmp/ttui" "$DIR/.ttui.new"
chmod 755 "$DIR/.ttui.new"
mv -f "$DIR/.ttui.new" "$DIR/ttui" # atomic, and safe while an old ttui is running
say "installed $("$DIR/ttui" --version) to $DIR/ttui"

case ":$PATH:" in
*":$DIR:"*) ;;
*)
	say ""
	say "$DIR is not on your PATH. Add it with:"
	case "${SHELL##*/}" in
	fish) say "  fish_add_path $DIR" ;;
	zsh) say "  echo 'export PATH=\"$DIR:\$PATH\"' >> ~/.zshrc" ;;
	*) say "  echo 'export PATH=\"$DIR:\$PATH\"' >> ~/.bashrc" ;;
	esac
	say "then open a new terminal."
	;;
esac
say "run: ttui"
