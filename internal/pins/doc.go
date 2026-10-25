// Package pins keeps the agreed host dependencies required in go.mod before the
// packages that use them exist. Its imports are compiled only with the "pins"
// build tag, so the host binary does not link them; building that tag with
// CGO_ENABLED=0 checks that each pinned dependency is pure Go.
package pins
