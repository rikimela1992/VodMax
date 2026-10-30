package service

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// messageCalls are the functions whose first string argument is a message
// for people: name → index of that argument.
var messageCalls = map[string]int{"invalid": 0, "Invalid": 0, "T": 1, "tr": 1, "New": 0}

// sourceMessages finds the literal messages passed to messageCalls in the
// Go files of dirs ("errors.New" only, not other New functions).
func sourceMessages(t *testing.T, dirs ...string) map[string]string {
	t.Helper()
	found := map[string]string{}
	fset := token.NewFileSet()
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, name, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var fn string
				switch x := call.Fun.(type) {
				case *ast.Ident:
					if x.Name == "New" {
						return true
					}
					fn = x.Name
				case *ast.SelectorExpr:
					pkg, ok := x.X.(*ast.Ident)
					if !ok || (x.Sel.Name == "New") != (pkg.Name == "errors") {
						return true
					}
					fn = x.Sel.Name
				}
				i, ok := messageCalls[fn]
				if !ok || len(call.Args) <= i {
					return true
				}
				lit, ok := call.Args[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				found[s] = fset.Position(lit.Pos()).String()
				return true
			})
		}
	}
	return found
}

var verbRE = regexp.MustCompile(`%[-+# 0]*(\d+|\*)?(\.(\d+|\*))?[a-zA-Z%]`)

func verbs(format string) []string {
	var out []string
	for _, v := range verbRE.FindAllString(format, -1) {
		if v != "%%" {
			out = append(out, v)
		}
	}
	return out
}

func TestMessagesTranslated(t *testing.T) {
	for msg, pos := range sourceMessages(t, ".", "../store", "../api") {
		if strings.Trim(verbRE.ReplaceAllString(msg, ""), " “”:;,.()") == "" {
			continue // only arguments, e.g. "%v"
		}
		if _, ok := messagesEN[msg]; !ok {
			t.Errorf("%s: no English for %q", pos, msg)
		}
	}
	for it, en := range messagesEN {
		if !slices.Equal(verbs(it), verbs(en)) {
			t.Errorf("verbs differ: %q → %q", it, en)
		}
	}
}

func TestLocalize(t *testing.T) {
	err := invalid("id non valido")
	if got := Localize("en", err); got != messagesEN["id non valido"] {
		t.Errorf("en: %q", got)
	}
	if got := Localize("it", err); got != "id non valido" {
		t.Errorf("it: %q", got)
	}
	busy := ConnectionBusy{Max: 2}
	if got := Localize("en", busy); !strings.Contains(got, "2") || got == busy.Error() {
		t.Errorf("busy: %q", got)
	}
}
