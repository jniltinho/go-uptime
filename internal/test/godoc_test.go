// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestGoDocCoverage fails when a package has no package comment or when an exported symbol has no doc comment, so that
// go doc stays complete. The doc comments are also what the description of the HTTP API is written from.
//
// A comment above the first name of a run of consecutive lines in a const or var block documents the whole run, which
// is how go doc shows it.
func TestGoDocCoverage(t *testing.T) {
	root := filepath.Join("..", "..")
	skipped := map[string]bool{"third_party": true, "node_modules": true, "vendor": true, "dist": true, ".git": true}
	packageHasDoc := map[string]bool{}
	var missing []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipped[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		directory := filepath.Dir(path)
		packageHasDoc[directory] = packageHasDoc[directory] || file.Doc != nil
		for _, name := range undocumentedSymbols(fileSet, file) {
			relative, _ := filepath.Rel(root, path)
			missing = append(missing, fmt.Sprintf("%s: %s", filepath.ToSlash(relative), name))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for directory, hasDoc := range packageHasDoc {
		if !hasDoc {
			relative, _ := filepath.Rel(root, directory)
			missing = append(missing, fmt.Sprintf("%s: no package comment", filepath.ToSlash(relative)))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("%d symbols or packages without a doc comment:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

// undocumentedSymbols returns the exported functions, methods of exported types, types, constants and variables of the
// file that have no doc comment, with their line.
func undocumentedSymbols(fileSet *token.FileSet, file *ast.File) []string {
	var names []string
	add := func(position token.Pos, name string) {
		names = append(names, fmt.Sprintf("line %d, %s", fileSet.Position(position).Line, name))
	}
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if !declaration.Name.IsExported() || declaration.Doc != nil {
				continue
			}
			name := declaration.Name.Name
			if declaration.Recv != nil && len(declaration.Recv.List) > 0 {
				receiver := declaration.Recv.List[0].Type
				if pointer, ok := receiver.(*ast.StarExpr); ok {
					receiver = pointer.X
				}
				if identifier, ok := receiver.(*ast.Ident); ok {
					if !identifier.IsExported() {
						continue
					}
					name = identifier.Name + "." + name
				}
			}
			add(declaration.Pos(), "func "+name)
		case *ast.GenDecl:
			coveredLine := -1
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					if spec.Name.IsExported() && declaration.Doc == nil && spec.Doc == nil {
						add(spec.Pos(), "type "+spec.Name.Name)
					}
				case *ast.ValueSpec:
					line := fileSet.Position(spec.Pos()).Line
					if spec.Doc != nil || spec.Comment != nil || line == coveredLine+1 {
						coveredLine = fileSet.Position(spec.End()).Line
						continue
					}
					if declaration.Doc != nil {
						continue
					}
					for _, name := range spec.Names {
						if name.IsExported() {
							add(name.Pos(), "value "+name.Name)
						}
					}
				}
			}
		}
	}
	return names
}
