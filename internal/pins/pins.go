//go:build pins

package pins

import (
	_ "github.com/blacktop/go-macho"
	_ "github.com/deploymenttheory/go-bindings-macosplatform/bindings/frameworks/appkit"
	_ "github.com/deploymenttheory/go-bindings-macosplatform/bindings/frameworks/iokit"
	_ "github.com/deploymenttheory/go-bindings-macosplatform/bindings/frameworks/virtualization"
	_ "golang.org/x/arch/arm64/arm64asm"
	_ "howett.net/plist"
)
