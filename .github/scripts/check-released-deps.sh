#!/usr/bin/env bash
# check-released-deps: verify that go.mod files of this repo's published
# modules reference only final, released versions of LaunchDarkly modules.
#
# Flags:
#   - pseudo-versions:     vX.Y.Z-0.<timestamp>-<sha> (a commit, not a release)
#   - tagged pre-releases: vX.Y.Z-rc1, vX.Y.Z-alpha.pub.N, ...
#   - `replace` of a LaunchDarkly module: any left-hand-side target, versioned
#     or not, whether single-line or in a `replace ( ... )` block, regardless
#     of the replacement; plus a non-final LaunchDarkly pin on a replace's
#     target side.
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
    # Track the parenthesized `replace ( ... )` block form: entries inside it
    # start with the module path, not the keyword. Update state before the
    # LaunchDarkly filter, so keyword-only block lines are not dropped first.
    $1 == "replace" && $2 == "(" { inReplace = 1; next }
    $1 == ")" && inReplace { inReplace = 0; next }
    $0 !~ /github\.com\/launchdarkly\// { next }
    {
      n = split($0, tok, /[[:space:]]+/)
      # Any replace of a LaunchDarkly module is a redirect away from the
      # released dependency, whatever the target. The left-hand path sits at
      # tok[1] inside a replace block and at tok[2] on a single-line
      # directive; check it before its optional version, so versioned
      # replaces, grouped or not, cannot slip through.
      left = inReplace ? tok[1] : (tok[1] == "replace" ? tok[2] : "")
      if (left ~ /^github\.com\/launchdarkly\//) {
        print file ": " $0 " (LaunchDarkly modules must not be replaced on release branches)"
        next
      }
      for (i = 1; i <= n; i++) {
        if (tok[i] !~ /^github\.com\/launchdarkly\//) continue
        path = tok[i]; j = i + 1
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
