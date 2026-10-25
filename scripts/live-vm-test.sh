#!/usr/bin/env bash
# Build a package's test binary, sign it ad hoc with the VM entitlements and
# run its live tests. The host must admit an ad hoc signed binary carrying
# private entitlements; `go-ios-vm host preflight` reports what it needs.
#
# Usage: scripts/live-vm-test.sh [package] [test pattern]
set -eu -o pipefail

pkg="${1:-./internal/host/vz}"
pattern="${2:-Live}"
root="$(cd "$(dirname "$0")/.." && pwd)"
bin="$(mktemp -d)/live.test"
trap 'rm -rf "$(dirname "$bin")"' EXIT

CGO_ENABLED=0 go test -c -o "$bin" "$pkg"
codesign --force --sign - --entitlements "$root/build/vm.entitlements" "$bin"
echo "signed $bin with build/vm.entitlements"
status=0
(cd "$root/$pkg" && GO_IOS_VM_TEST_TIER=live "$bin" -test.run "$pattern" -test.v -test.count 1 -test.timeout 30m) || status=$?
if [ "$status" -ne 0 ]; then
	echo "live VM test exited with status $status" >&2
fi
exit "$status"
