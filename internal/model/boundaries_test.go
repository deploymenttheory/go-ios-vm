package model

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPackageDependencies(t *testing.T) {
	// This is an architectural gate, not a snapshot of today's import lists.
	allowed := map[string][]string{
		"model":    {},
		"workflow": {"github.com/deploymenttheory/go-ios-vm/internal/model"},
	}
	for pkg, external := range allowed {
		files, err := os.ReadDir(filepath.Join("..", pkg))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range files {
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", pkg, entry.Name()), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, i := range f.Imports {
				path, _ := strconv.Unquote(i.Path.Value)
				if !strings.Contains(strings.Split(path, "/")[0], ".") {
					continue
				}
				ok := false
				for _, a := range external {
					if path == a {
						ok = true
					}
				}
				if !ok {
					t.Errorf("%s must not depend on %s", pkg, path)
				}
			}
		}
	}
	for _, pkg := range []string{"acquisition", "inspection", "postgres"} {
		files, err := os.ReadDir(filepath.Join("..", pkg))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range files {
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", pkg, entry.Name()), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, i := range f.Imports {
				path, _ := strconv.Unquote(i.Path.Value)
				for _, forbidden := range []string{"github.com/spf13/", "github.com/deploymenttheory/go-ios-vm/cmd/", "github.com/deploymenttheory/go-ios-vm/internal/workflow"} {
					if strings.HasPrefix(path, forbidden) {
						t.Errorf("%s imports higher-level dependency %s", pkg, path)
					}
				}
				for _, adapter := range []string{"acquisition", "inspection", "postgres"} {
					if adapter != pkg && path == "github.com/deploymenttheory/go-ios-vm/internal/"+adapter {
						t.Errorf("adapter %s depends on adapter %s", pkg, adapter)
					}
				}
			}
		}
	}
}
