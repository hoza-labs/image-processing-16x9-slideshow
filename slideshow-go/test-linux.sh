#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
export CGO_ENABLED=0
go vet ./...
xvfb-run -a -s '-screen 0 1280x720x24' bash -c '
  openbox >/tmp/slideshow-openbox.log 2>&1 &
  wm=$!
  trap "kill $wm 2>/dev/null || true" EXIT
  SLIDESHOW_GUI_TEST=1 go test -count=1 -timeout 60s -v ./...
'
go build -trimpath -o /tmp/slideshow-linux .
if readelf -l /tmp/slideshow-linux | grep -q INTERP; then
  echo 'Unexpected dynamic loader in Linux executable' >&2
  exit 1
fi
/tmp/slideshow-linux --help
