#!/usr/bin/env bash
set -e

VERSION="${1:-v1.0.0}"
DIST_DIR="$(cd "$(dirname "$0")/.." && pwd)/dist"
BIN_DIR="$(cd "$(dirname "$0")/.." && pwd)/bin"
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

rm -rf "$DIST_DIR" "$BIN_DIR"
mkdir -p "$DIST_DIR" "$BIN_DIR"

echo "Building yt-import $VERSION for all platforms..."

TARGETS=(
  "windows/amd64/.exe"
  "windows/arm64/.exe"
  "darwin/amd64/"
  "darwin/arm64/"
  "linux/amd64/"
  "linux/arm64/"
)

for target in "${TARGETS[@]}"; do
  IFS="/" read -r GOOS GOARCH EXT <<< "$target"
  OUT_NAME="yt-import${EXT}"
  ASSET_NAME="yt-import_${VERSION}_${GOOS}_${GOARCH}"

  echo "  -> Building ${GOOS}/${GOARCH}..."
  CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w" -trimpath -o "${BIN_DIR}/${OUT_NAME}" ./cmd/yt-import

  cd "$BIN_DIR"
  if [ "$GOOS" = "windows" ]; then
    zip -j "${DIST_DIR}/${ASSET_NAME}.zip" "${OUT_NAME}" "${ROOT_DIR}/README.md" "${ROOT_DIR}/LICENSE"
  else
    tar -czf "${DIST_DIR}/${ASSET_NAME}.tar.gz" -C "${BIN_DIR}" "${OUT_NAME}" -C "${ROOT_DIR}" README.md LICENSE
  fi
  rm -f "${BIN_DIR}/${OUT_NAME}"
  cd "$ROOT_DIR"
done

rm -rf "$BIN_DIR"

cd "$DIST_DIR"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum * > checksums.txt
elif command -v shasum >/dev/null 2>&1; then
  shasum -a 256 * > checksums.txt
fi

echo "Build complete! Archives available in ./dist:"
ls -lh "$DIST_DIR"
