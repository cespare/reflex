#!/bin/bash
set -eu -o pipefail

cd "$(dirname "$0")"

version="$1"

if [[ "$(git rev-parse "${version}^{commit}")" != "$(git rev-parse HEAD)" ]]; then
  echo "Not currently on version $version" 2>&1
  exit 1
fi

rm -rf release
mkdir release

for goos in linux darwin; do
  for goarch in amd64 arm64; do
    dir="release/reflex_${goos}_${goarch}"
    mkdir "$dir"
    cp LICENSE "${dir}/LICENSE"
    GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build -o "${dir}/reflex"
    tar -c -f - -C release "$(basename "$dir")" | gzip -9 >"${dir}.tar.gz"
    rm -rf "${dir}"
    sha256sum "${dir}.tar.gz" >"${dir}.tar.gz.sha256"
  done
done

exec gh release create "$version" \
  --title "Reflex ${version#v}" \
  ./release/*.tar.gz \
  ./release/*.tar.gz.sha256
