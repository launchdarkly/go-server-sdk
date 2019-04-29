#!/usr/bin/env bash
# This script updates the version for the ldclient.go

set -uxe
echo "Starting go-server-sdk release."

VERSION=$1

#Update version in ldclient.go
LDCLIENT_GO_TEMP=./ldclient.go.tmp
sed "s/const Version =.*/const Version = \"${VERSION}\"/g"  ldclient.go > ${LDCLIENT_GO_TEMP}
mv ${LDCLIENT_GO_TEMP} ldclient.go

echo "Done with go-server-sdk release"

