#!/bin/sh
# garlic installer — downloads the latest release binary, verifies its
# checksum, and installs it.
#
#   curl -fsSL https://raw.githubusercontent.com/lucky7xz/garlic/main/install.sh | sh
#
# Pure POSIX sh. Installs to ~/.local/bin (or /usr/local/bin when writable).

set -eu

REPO="lucky7xz/garlic"
BIN="garlic"

err() { echo "garlic-install: $*" >&2; exit 1; }

# --- pick a downloader -------------------------------------------------------
if command -v curl >/dev/null 2>&1; then
	dl() { curl -fsSL "$1"; }
	dlf() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
	dl() { wget -qO- "$1"; }
	dlf() { wget -qO "$2" "$1"; }
else
	err "need curl or wget"
fi

# --- detect OS + arch, map to the release archive name -----------------------
# GoReleaser names archives garlic_<Os>_<Arch>.tar.gz, e.g. garlic_Linux_x86_64.
os=$(uname -s)
case "$os" in
	Linux)   OS=Linux ;;
	Darwin)  OS=Darwin ;;
	FreeBSD) OS=Freebsd ;;
	*)       err "unsupported OS: $os" ;;
esac

arch=$(uname -m)
case "$arch" in
	x86_64 | amd64)  ARCH=x86_64 ;;
	aarch64 | arm64) ARCH=arm64 ;;
	armv7* | armv6*) ARCH=armv7 ;;
	*)               err "unsupported architecture: $arch" ;;
esac

ASSET="${BIN}_${OS}_${ARCH}.tar.gz"

# --- resolve the latest release tag ------------------------------------------
TAG=$(dl "https://api.github.com/repos/${REPO}/releases/latest" \
	| grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name" *: *"([^"]+)".*/\1/')
[ -n "$TAG" ] || err "could not determine the latest release"

BASE="https://github.com/${REPO}/releases/download/${TAG}"
echo "garlic-install: ${TAG} for ${OS}/${ARCH}"

# --- download archive + checksums into a temp dir ----------------------------
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

dlf "${BASE}/${ASSET}" "${TMP}/${ASSET}" || err "download failed: ${ASSET}"
dlf "${BASE}/checksums.txt" "${TMP}/checksums.txt" || err "download failed: checksums.txt"

# --- verify checksum (mandatory) ---------------------------------------------
if command -v sha256sum >/dev/null 2>&1; then
	sumcmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
	sumcmd="shasum -a 256"
else
	err "need sha256sum or shasum to verify the download"
fi

want=$(grep " ${ASSET}\$" "${TMP}/checksums.txt" | awk '{print $1}')
[ -n "$want" ] || err "no checksum listed for ${ASSET}"
got=$(cd "$TMP" && $sumcmd "$ASSET" | awk '{print $1}')
[ "$want" = "$got" ] || err "checksum mismatch for ${ASSET} (expected ${want}, got ${got})"

# --- unpack and install ------------------------------------------------------
tar -xzf "${TMP}/${ASSET}" -C "$TMP" "$BIN" || err "could not extract ${BIN}"

if [ -w /usr/local/bin ] || [ "$(id -u)" = "0" ]; then
	DEST="/usr/local/bin"
else
	DEST="${HOME}/.local/bin"
	mkdir -p "$DEST"
fi

install -m 0755 "${TMP}/${BIN}" "${DEST}/${BIN}" || err "could not install to ${DEST}"
echo "garlic-install: installed ${DEST}/${BIN}"
"${DEST}/${BIN}" version 2>/dev/null || true

# --- PATH hint ---------------------------------------------------------------
case ":${PATH}:" in
	*":${DEST}:"*) ;;
	*) echo "garlic-install: add ${DEST} to your PATH:  export PATH=\"${DEST}:\$PATH\"" ;;
esac
