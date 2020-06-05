#!/usr/bin/env bash
# This script updates the version for the ldclient.go

set -ue

SOURCE_FILE=./internal/version.go
TEMP_FILE=${SOURCE_FILE}.tmp
sed "s/const SDKVersion =.*/const SDKVersion = \"${LD_RELEASE_VERSION}\"/g" ${SOURCE_FILE} > ${TEMP_FILE}
mv ${TEMP_FILE} ${SOURCE_FILE}
