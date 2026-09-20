#!/usr/bin/env bash
set -euo pipefail

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$root/slideshow-go"
command -v go >/dev/null 2>&1 || { echo 'Go 1.24 or newer is required on PATH.' >&2; exit 1; }
export CGO_ENABLED=0
mkdir -p dist
# Keep the embedded license synchronized with the repository license.
cp "$root/LICENSE" LICENSE
# Run tests on the build host before cross-compiling all deliverables.
env -u GOOS -u GOARCH go test ./...
for target in windows linux darwin; do
  suffix=''
  [[ "$target" != windows ]] || suffix='.exe'
  for arch in amd64 arm64; do
    GOOS="$target" GOARCH="$arch" go build -trimpath -ldflags='-s -w' \
      -o "dist/slideshow-${target}-${arch}${suffix}" .
  done
done
cp "$root/LICENSE" THIRD_PARTY_NOTICES.txt dist/
echo "Built Windows, Linux, and macOS (amd64 and arm64) executables in $root/slideshow-go/dist"
