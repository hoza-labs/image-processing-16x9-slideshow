#!/usr/bin/env bash
set -eu

apt update -y
apt install -y imagemagick

imagemagick --version
