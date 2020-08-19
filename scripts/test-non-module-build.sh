#!/bin/bash

set -eu

# This verifies that it is possible to build a Go application that uses the SDK when modules are not enabled--
# meaning that the SDK does not have any module-only dependencies (i.e. dependencies that are modules with a
# /vN major version suffix in the import path). We use govendor to verify this, since it never understands
# modules whereas "go get" can understand module paths even in a non-module project.
#
# This test assumes that the main entry point, gopkg.in/launchdarkly/go-server-sdk, imports all other SDK
# packages (including ones from go-sdk-common) either directly or indirectly, not including ones that are not
# used in normal usage of the SDK such as testhelpers/storetest.

SDK_DIR=$(pwd)
PACKAGE_PATH=gopkg.in/launchdarkly/go-server-sdk.v5

TEMP_DIR=$(mktemp -d -t go-sdk-XXXXXXXXX)
trap "rm -rf ${TEMP_DIR}" EXIT

export GOPATH=${TEMP_DIR}/go
export PATH=${GOPATH}/bin:${PATH}
export GO111MODULE=off

mkdir -p ${GOPATH}
mkdir -p ${GOPATH}/src/${PACKAGE_PATH}
cp -r ${SDK_DIR}/* ${GOPATH}/src/${PACKAGE_PATH}

mkdir -p ${GOPATH}/src/testapp
cd ${GOPATH}/src/testapp

go get -u github.com/kardianos/govendor
govendor init
govendor add -tree ${PACKAGE_PATH}  # adds SDK from GOPATH
govendor fetch +missing             # fetches all other transitive dependencies with git

cat >main.go <<EOF
package main
import ld "gopkg.in/launchdarkly/go-server-sdk.v5"
func main() {
    _, _ = ld.MakeCustomClient("sdk-key", ld.Config{}, 0)
}
EOF

go build

echo "Successfully built test app using the SDK without modules"
