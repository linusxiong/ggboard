package httpx

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestEveryErrorMessageIsTranslated scans internal/ for the messages that reach API responses and checks that
// each has a zh-CN translation. A message is found where it is written:
//
//   - the message argument of the httpx constructors (BadRequest, FieldError, Conflict, ...) and of WithField;
//   - the field-error helpers of the handlers and services: .add(field, msg), mail's addField, account's
//     retryError and payment gateways' UserError{Msg: ...};
//   - the validators whose errors become field messages: settings (fmt.Errorf / errors.New / errs[k] = append(
//     errs[k], literal)), the content errors of internal/api/admin (var errX = errors.New), mail's const errX, and the
//     returns of httpx's message/typeMessage and admin's mailTemplateLocaleError.
//
// A message built with fmt.Sprintf or "+" is checked with 7 in place of each dynamic part, so the catalog needs
// a pattern for it. Other dynamic messages (err.Error() of a dependency) are not checked here.
func TestEveryErrorMessageIsTranslated(t *testing.T) {
	msgs := scanMessages(t, filepath.Join("..", ""))
	if len(msgs) < 200 {
		t.Fatalf("scan found only %d messages; the scanner is broken", len(msgs))
	}
	var missing []string
	for sample, where := range msgs {
		if Translate(LocaleZH, sample) == sample {
			missing = append(missing, strconv.Quote(sample)+"  // "+where)
		}
	}
	slices.Sort(missing)
	if len(missing) > 0 {
		t.Errorf("%d messages have no zh-CN translation (add them to messagesZH):\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

var (
	ctorArg = map[string]int{
		"BadRequest": 0, "FieldError": 1, "Forbidden": 0, "NotFound": 0, "Conflict": 0, "Unauthorized": 0,
		"PaymentFailed": 0, "PayloadTooLarge": 0, "UnsupportedMediaType": 0, "RateLimited": 0, "NewError": 2,
	}
	returnFuncs = map[string]bool{"message": true, "typeMessage": true, "mailTemplateLocaleError": true}
	verbRe      = regexp.MustCompile(`%[-+# 0]*[0-9]*(\.[0-9]+)?[a-zA-Z]`)
)

// scanMessages returns sample message → first place it is written.
func scanMessages(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		slash := filepath.ToSlash(path)
		inSettings := strings.Contains(slash, "/settings/")
		inAdminAPI := strings.Contains(slash, "/api/admin/")
		inMail := strings.HasSuffix(filepath.Dir(slash), "/mail")
		add := func(e ast.Expr) {
			if s, ok := sample(e); ok && strings.TrimSpace(s) != "" {
				if _, seen := out[s]; !seen {
					p := fset.Position(e.Pos())
					out[s] = filepath.ToSlash(p.Filename) + ":" + strconv.Itoa(p.Line)
				}
			}
		}
		arg := func(call *ast.CallExpr, i int) {
			if i < len(call.Args) {
				add(call.Args[i])
			}
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || (gd.Tok != token.VAR && gd.Tok != token.CONST) {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i >= len(vs.Values) || !strings.HasPrefix(name.Name, "err") {
						continue
					}
					switch v := vs.Values[i].(type) {
					case *ast.BasicLit:
						if inMail && gd.Tok == token.CONST {
							add(v)
						}
					case *ast.CallExpr:
						if inAdminAPI && isCall(v, "errors", "New") {
							arg(v, 0)
						}
					}
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncDecl:
				if returnFuncs[n.Name.Name] && n.Body != nil {
					ast.Inspect(n.Body, func(m ast.Node) bool {
						if r, ok := m.(*ast.ReturnStmt); ok && len(r.Results) == 1 {
							add(r.Results[0])
						}
						return true
					})
				}
			case *ast.CompositeLit:
				if sel, ok := n.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "UserError" {
					for _, el := range n.Elts {
						if kv, ok := el.(*ast.KeyValueExpr); ok {
							if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Msg" {
								add(kv.Value)
							}
						}
					}
				}
			case *ast.CallExpr:
				switch fn := n.Fun.(type) {
				case *ast.SelectorExpr:
					if x, ok := fn.X.(*ast.Ident); ok && x.Name == "httpx" {
						if i, ok := ctorArg[fn.Sel.Name]; ok {
							arg(n, i)
						}
					}
					switch fn.Sel.Name {
					case "WithField":
						if len(n.Args) == 2 && !isString(n.Args[0], "trade_no") {
							arg(n, 1)
						}
					case "add":
						if len(n.Args) == 2 {
							arg(n, 1)
						}
					}
					if inSettings && (isCall(n, "fmt", "Errorf") || isCall(n, "errors", "New")) {
						if s, ok := sample(n); ok && !strings.HasPrefix(s, "settings:") && !strings.Contains(s, "%w") {
							add(n)
						}
					}
				case *ast.Ident:
					if i, ok := ctorArg[fn.Name]; ok && f.Name.Name == "httpx" {
						arg(n, i)
					}
					switch fn.Name {
					case "addField":
						arg(n, 2)
					case "retryError":
						arg(n, 0)
					case "append":
						if _, ok := n.Args[0].(*ast.IndexExpr); ok && inSettings && len(n.Args) == 2 {
							arg(n, 1) // errs[k] = append(errs[k], "message")
						}
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// sample renders a message expression with 7 for every dynamic part; ok is false when the whole message is
// dynamic (a variable or a call such as err.Error()).
func sample(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.ParenExpr:
		return sample(e.X)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		l, lok := sample(e.X)
		r, rok := sample(e.Y)
		if !lok && !rok {
			return "", false
		}
		if !lok {
			l = "7"
		}
		if !rok {
			r = "7"
		}
		return l + r, true
	case *ast.CallExpr:
		if (isCall(e, "fmt", "Sprintf") || isCall(e, "fmt", "Errorf") || isCall(e, "errors", "New")) && len(e.Args) > 0 {
			s, ok := sample(e.Args[0])
			if !ok {
				return "", false
			}
			if isCall(e, "errors", "New") {
				return s, true
			}
			return strings.ReplaceAll(verbRe.ReplaceAllString(strings.ReplaceAll(s, "%%", "\x00"), "7"), "\x00", "%"), true
		}
	}
	return "", false
}

func isCall(c *ast.CallExpr, pkg, name string) bool {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg && sel.Sel.Name == name
}

func isString(e ast.Expr, s string) bool {
	bl, ok := e.(*ast.BasicLit)
	return ok && bl.Kind == token.STRING && bl.Value == strconv.Quote(s)
}
