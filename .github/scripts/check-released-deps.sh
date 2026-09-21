#!/usr/bin/env bash
# check-released-deps: verify that go.mod files of this repo's published
# modules reference only final, released versions of LaunchDarkly modules.
#
# Flags:
#   - pseudo-versions:     vX.Y.Z-0.<timestamp>-<sha> (a commit, not a release)
#   - tagged pre-releases: vX.Y.Z-rc1, vX.Y.Z-alpha.pub.N, ...
#   - `replace` directives that redirect a LaunchDarkly module to a directory
#     or to an unreleased version.
#
# Non-LaunchDarkly dependencies are not validated: third-party modules
# legitimately use pseudo-versions and +incompatible suffixes.
#
# The unpublished testservice module is excluded by the caller; its committed
# `replace => ../` of the SDK is expected.
#
# Usage: check-released-deps.sh <go.mod> [...]
# Exit 0 if every LaunchDarkly reference is a released version; 1 otherwise.

set -u

rc=0
for f in "$@"; do
  findings=$(awk -v file="$f" '
    { sub(/\/\/.*/, ""); gsub(/^[[:space:]]+|[[:space:]]+$/, "") }   # strip comments/indent
    $1 == "module" || $1 == "go" || $1 == "toolchain" || $1 == "retract" { next }
    $0 !~ /github\.com\/launchdarkly\// { next }
    {
      n = split($0, tok, /[[:space:]]+/)
      for (i = 1; i <= n; i++) {
        if (tok[i] !~ /^github\.com\/launchdarkly\//) continue
        path = tok[i]; j = i + 1
        if (j <= n && tok[j] == "=>") {                # replace redirect
          j++
          if (j <= n && tok[j] ~ /^github\.com\/launchdarkly\//) j++
          if (j > n || tok[j] !~ /^v[0-9]+\.[0-9]+\.[0-9]+$/) print file ": " $0 " (not a released version)"
          break
        }
        if (j > n || tok[j] !~ /^v[0-9]+\.[0-9]+\.[0-9]+$/) {
          print file ": " path " " (j <= n ? tok[j] : "(no version)") " (not a released version)"
        }
      }
    }
  ' "$f")
  if [ -n "$findings" ]; then
    rc=1
    printf '%s\n' "$findings"
  fi
done

if [ "$rc" -ne 0 ]; then
  echo "FAIL: LaunchDarkly component modules listed above are not pinned to released versions." >&2
  echo "Update go.mod to released versions before merging to a release branch." >&2
fi
exit $rc
