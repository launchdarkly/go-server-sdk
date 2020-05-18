#!/usr/bin/env bash
# This script updates the version for the ldclient.go

set -uxe
echo "Starting go-server-sdk release."

VERSION=$1

SOURCE_FILE=./internal/version.go
TEMP_FILE=${SOURCE_FILE}.tmp
sed "s/const SDKVersion =.*/const SDKVersion = \"${VERSION}\"/g" ${SOURCE_FILE} > ${TEMP_FILE}
mv ${TEMP_FILE} ${SOURCE_FILE}

echo "Done with go-server-sdk release"

