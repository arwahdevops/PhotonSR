#!/bin/bash
# build.sh

set -e

VERSION="0.2.0"
COMMIT_HASH=$(git rev-parse --short HEAD)
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
BUILT_BY="build-script"

LDFLAGS="-X 'main.version=$VERSION' \
         -X 'main.commit=$COMMIT_HASH' \
         -X 'main.date=$BUILD_DATE' \
         -X 'main.builtBy=$BUILT_BY'"

echo "Building PhotonSR version $VERSION..."
go build -ldflags="$LDFLAGS" -o photonsr ./cmd
echo "Build complete: ./photonsr"
echo "Verifying version:"
./photonsr -version
