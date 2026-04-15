package anthropic_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestAllMarshalerTypesImplementDirectEncoder parses every non-test Go
// source file in the package and asserts that every type with a
// MarshalJSON method also has an EncodeDirect method. This catches new
// types that forget to implement the DirectEncoder fast path.
func TestAllMarshalerTypesImplementDirectEncoder(t *testing.T) {
	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	marshalers := map[string]bool{}
	directEncoders := map[string]bool{}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}
			typeName := receiverTypeName(fn.Recv.List[0].Type)
			if typeName == "" {
				continue
			}
			switch fn.Name.Name {
			case "MarshalJSON":
				marshalers[typeName] = true
			case "EncodeDirect":
				directEncoders[typeName] = true
			}
		}
	}

	var missing []string
	for typeName := range marshalers {
		if !directEncoders[typeName] {
			missing = append(missing, typeName)
		}
	}
	sort.Strings(missing)

	for _, typeName := range missing {
		t.Errorf("type %s implements MarshalJSON but not EncodeDirect", typeName)
	}
	if len(missing) > 0 {
		t.Logf("%d type(s) missing EncodeDirect out of %d with MarshalJSON",
			len(missing), len(marshalers))
	}
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}
