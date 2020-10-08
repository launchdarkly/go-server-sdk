#!/bin/bash

# By default, Go projects use a "v" prefix for release tags ("v5.0.0"). This is
# required for Go modules.
#
# We are still supporting non-module usage of the SDK, with other package managers
# such as dep. Users of those tools may not be using a "v" tag prefix, so we will
# continue to create release tags without the prefix as well, until we move to
# only supporting module usage.

echo "v${LD_RELEASE_VERSION}"
echo "${LD_RELEASE_VERSION}"
