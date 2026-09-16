#!/usr/bin/env bash
set -euo pipefail

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$root/slideshow-go"
command -v go >/dev/null 2>&1 || { echo 'Go 1.24 or newer is required on PATH.' >&2; exit 1; }
export CGO_ENABLED=0
arch="${GOARCH:-amd64}"
mkdir -p dist
# Run tests on the build host before cross-compiling both deliverables.
env -u GOOS -u GOARCH go test ./...
for target in windows linux; do
  suffix=''
  [[ "$target" != windows ]] || suffix='.exe'
  GOOS="$target" GOARCH="$arch" go build -trimpath -ldflags='-s -w' \
    -o "dist/slideshow-${target}-${arch}${suffix}" .
done
cp THIRD_PARTY_NOTICES.txt dist/
echo "Built Windows and Linux ($arch) executables in $root/slideshow-go/dist"
