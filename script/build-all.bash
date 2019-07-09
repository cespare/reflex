#!/usr/bin/env bash
# Copyright 2017 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#
# This script will build reflex and calculate hash for each
# (REFLEX_BUILD_PLATFORMS, REFLEX_BUILD_ARCHS) pair.
# REFLEX_BUILD_PLATFORMS="linux" REFLEX_BUILD_ARCHS="amd64" ./script/build-all.bash
# can be called to build only for linux-amd64

set -e

REFLEX_ROOT=$(git rev-parse --show-toplevel)
VERSION=$(git describe --tags --dirty)
COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null)
DATE=$(date "+%Y-%m-%d")
BUILD_PLATFORM=$(uname -a | awk '{print tolower($1);}')
IMPORT_DURING_SOLVE=${IMPORT_DURING_SOLVE:-false}

if [[ "$(pwd)" != "${REFLEX_ROOT}" ]]; then
  echo "you are not in the root of the repo" 1>&2
  echo "please cd to ${REFLEX_ROOT} before running this script" 1>&2
  exit 1
fi

GO_BUILD_CMD="go build -a -installsuffix cgo"
GO_BUILD_LDFLAGS="-s -w -X main.commitHash=${COMMIT_HASH} -X main.buildDate=${DATE} -X main.version=${VERSION} -X main.flagImportDuringSolve=${IMPORT_DURING_SOLVE}"

if [[ -z "${REFLEX_BUILD_PLATFORMS}" ]]; then
    REFLEX_BUILD_PLATFORMS="linux darwin freebsd"
fi

if [[ -z "${REFLEX_BUILD_ARCHS}" ]]; then
    REFLEX_BUILD_ARCHS="amd64 386 ppc64 ppc64le s390x arm arm64"
fi

mkdir -p "${REFLEX_ROOT}/release"

for OS in ${REFLEX_BUILD_PLATFORMS[@]}; do
  for ARCH in ${REFLEX_BUILD_ARCHS[@]}; do
    NAME="reflex-${OS}-${ARCH}"
    if [[ "${OS}" == "windows" ]]; then
      NAME="${NAME}.exe"
    fi


    CGO_ENABLED=0

    if [[ "${ARCH}" == "ppc64" || "${ARCH}" == "ppc64le" || "${ARCH}" == "s390x" || "${ARCH}" == "arm" || "${ARCH}" == "arm64" ]] && [[ "${OS}" != "linux" ]]; then
        # ppc64, ppc64le, s390x, arm and arm64 are only supported on Linux.
        echo "Building for ${OS}/${ARCH} not supported."
    else
        echo "Building for ${OS}/${ARCH} with CGO_ENABLED=${CGO_ENABLED}"
        GOARCH=${ARCH} GOOS=${OS} CGO_ENABLED=${CGO_ENABLED} ${GO_BUILD_CMD} -ldflags "${GO_BUILD_LDFLAGS}"\
            -o "${REFLEX_ROOT}/release/${NAME}"
        pushd "${REFLEX_ROOT}/release" > /dev/null
        shasum -a 256 "${NAME}" > "${NAME}.sha256"
        popd > /dev/null
    fi
  done
done