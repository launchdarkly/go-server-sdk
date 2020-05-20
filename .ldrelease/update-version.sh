#!/usr/bin/env bash
# This script updates the version for the ldclient.go

set -ue

LDCLIENT_GO_TEMP=./ldclient.go.tmp
sed "s/const Version =.*/const Version = \"${LD_RELEASE_VERSION}\"/g" ldclient.go > ${LDCLIENT_GO_TEMP}
mv ${LDCLIENT_GO_TEMP} ldclient.go
