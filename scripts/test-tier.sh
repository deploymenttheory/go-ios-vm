#!/usr/bin/env bash
# Run the native or live test tier and fail unless at least one test actually
# ran in it, so an empty or fully skipped tier never reads as verified.
#
# Usage: scripts/test-tier.sh native|live [packages...]
set -eu -o pipefail

tier="${1:-}"
case "$tier" in
native | live) ;;
*)
	echo "usage: $0 native|live [packages...]" >&2
	exit 2
	;;
esac
shift
if [ "$#" -eq 0 ]; then
	set -- ./...
fi

out="$(mktemp)"
trap 'rm -f "$out"' EXIT

status=0
GO_IOS_VM_TEST_TIER="$tier" go test -count=1 -v "$@" | tee "$out" || status=$?
ran="$(grep -c "testenv: running in tier $tier\$" "$out" || true)"

if [ "$status" -ne 0 ]; then
	echo "$tier tier: tests failed or prerequisites are unmet (exit $status)" >&2
	exit "$status"
fi
if [ "$ran" -eq 0 ]; then
	echo "$tier tier: no test ran in this tier; nothing was verified" >&2
	exit 1
fi
echo "$tier tier: $ran test(s) ran"
