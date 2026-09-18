#!/usr/bin/env bash
set -eu

docker run --rm -it \
  -v "$(cygpath -w "$(pwd)"):/app" \
  -w //app \
  ubuntu:26.04
  bash

