#!/bin/bash
# Pack the Persona extension: dist/personal-agent-club-<version>.tar.gz and its .sha256.
#
# The archive holds one top-level directory with the manifest (as persona-extension.json, and as persona-plugin.json
# for hubs from before the rename to extensions) and the program for each platform the manifest names, built with
# -trimpath so no path of the machine that built it is inside. No source, no tests, nothing else.
#
# Install it on the hub's machine: personad extension install file:/path/to/personal-agent-club-<version>.tar.gz
set -euo pipefail
cd "$(dirname "$0")/.."
GO=${GO:-go}
id=personal-agent-club
version=$(python3 -I -c 'import json,sys;print(json.load(open(sys.argv[1]))["version"])' persona-extension.json)
ldflags="-s -w"
if [ -n "${VERSION:-}" ]; then
  [ "$VERSION" = "$version" ] || { echo "VERSION=$VERSION but the manifest says $version: the hub wants them equal" >&2; exit 1; }
fi

out=dist/$id
rm -rf "$out" "dist/$id-$version.tar.gz" "dist/$id-$version.tar.gz.sha256"
mkdir -p "$out/bin"
cp persona-extension.json "$out/persona-extension.json"
cp persona-extension.json "$out/persona-plugin.json"

python3 -I -c 'import json,sys;[print(k, v) for k, v in json.load(open(sys.argv[1]))["runtime"]["command"].items()]' persona-extension.json |
while read -r plat path; do
  echo "build $plat -> $path"
  CGO_ENABLED=0 GOOS=${plat%-*} GOARCH=${plat#*-} "$GO" build -trimpath -buildvcs=false -ldflags "$ldflags" \
    -o "$out/$path" ./cmd/personal-agent-club
done

# Nothing of this machine in the programs: not the checkout, not the home directory, not the module cache.
for leak in "$PWD" "$HOME" "$("$GO" env GOMODCACHE)"; do
  if grep -rlF -- "$leak" "$out" >/dev/null; then
    echo "a packaged file contains a local path; refusing to pack" >&2
    exit 1
  fi
done

# No AppleDouble files or extended attributes (a stray ._<id> at the top level hides the manifest from the hub), and
# no owner of this machine in the headers.
if tar --version 2>/dev/null | grep -q bsdtar; then
  owner=(--uid 0 --gid 0 --uname root --gname root)
else
  owner=(--owner=0 --group=0 --numeric-owner)
fi
COPYFILE_DISABLE=1 tar --no-xattrs "${owner[@]}" --exclude='._*' --exclude=.DS_Store -C dist -czf "dist/$id-$version.tar.gz" "$id"
if command -v sha256sum >/dev/null; then
  (cd dist && sha256sum "$id-$version.tar.gz" | tee "$id-$version.tar.gz.sha256")
else
  (cd dist && shasum -a 256 "$id-$version.tar.gz" | tee "$id-$version.tar.gz.sha256")
fi
tar -tzvf "dist/$id-$version.tar.gz"
