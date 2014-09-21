#!/bin/bash
godep go install -ldflags "-X github.com/launchdarkly/ldclient.Version `git rev-parse HEAD | cut -c1-6`"