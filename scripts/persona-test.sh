#!/bin/bash
# Hold the program to the hub's own checks: `personad extension test` (manifest and package as at install, then the
# handshake, error codes, snake_case and every ui contribution drawn as the apps can draw it). It needs no running
# hub, only a personad (PERSONAD, default the one on PATH).
#
# `personad extension test <dir>` builds the Go main package in <dir>, and this repository's main package is
# cmd/personal-agent-club, so the check runs on a scratch copy of the module with the manifest next to it.
set -euo pipefail
cd "$(dirname "$0")/.."
personad=${PERSONAD:-personad}
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
mkdir -p "$scratch/src"
git ls-files -z --cached --others --exclude-standard | tar -c --null -T - | tar -x -C "$scratch/src"
cp persona-extension.json "$scratch/src/cmd/personal-agent-club/"
mkdir -p "$scratch/tmp" "$scratch/home"
# A home and temporary directory of its own: the check writes its scratch data there, and reads nothing of a hub.
env -i PATH="$PATH" HOME="$scratch/home" TMPDIR="$scratch/tmp" GOPATH="$(go env GOPATH)" GOCACHE="$(go env GOCACHE)" \
  GOTOOLCHAIN="$(go env GOTOOLCHAIN)" \
  "$personad" extension test "$scratch/src/cmd/personal-agent-club"
