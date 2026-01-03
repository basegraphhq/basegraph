package golang

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/humanbeeng/lepo/prototypes/codegraph/extract"
)

type FunctionVisitor struct {
	Functions map[string]extract.Function
	Fset      *token.FileSet
	Info      *types.Info
}

func (v *FunctionVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	switch n := node.(type) {

	// TODO: Add FuncType which is suspected to be in interface
	case *ast.FuncDecl:
		{
			fnObj, ok := v.Info.Defs[n.Name]
			if !ok {
				return v
			}

			pos := v.Fset.Position(n.Pos()).Line
			end := v.Fset.Position(n.End()).Line
			filepath := v.Fset.Position(n.Pos()).Filename
			namespace := extract.Namespace{Name: fnObj.Pkg().Path()}
			qname := namespace.Name + "." + fnObj.Name()

			mCode, err := extractCode(n, v.Fset)
			if err != nil {
				// TODO: Better error handling
				panic(err)
			}

			if n.Recv != nil {
				for _, field := range n.Recv.List {
					if id, ok := field.Type.(*ast.Ident); ok {
						// Regular method - qname includes type: pkg.Type.Method
						typeName := id.Name
						stQName := fnObj.Pkg().Path() + "." + typeName
						methodQName := stQName + "." + fnObj.Name()

						f := extract.Function{
							Name:        fnObj.Name(),
							QName:       methodQName,
							Namespace:   namespace,
							ParentQName: stQName,
							Pos:         pos,
							End:         end,
							Filepath:    filepath,
							Code:        mCode,
						}

						v.extractParamsAndReturns(n, &f)
						v.extractDoc(n, &f)

						v.Functions[methodQName] = f
						qname = methodQName // Update for body visitor

					} else if se, ok := field.Type.(*ast.StarExpr); ok {
						// Pointer based method - qname includes type: pkg.Type.Method
						if id, ok := se.X.(*ast.Ident); ok {
							typeName := id.Name
							stQName := fnObj.Pkg().Path() + "." + typeName
							methodQName := stQName + "." + fnObj.Name()

							f := extract.Function{
								Name:        fnObj.Name(),
								QName:       methodQName,
								Namespace:   namespace,
								ParentQName: stQName,
								Pos:         pos,
								End:         end,
								Filepath:    filepath,
								Code:        mCode,
							}

							v.extractParamsAndReturns(n, &f)
							v.extractDoc(n, &f)

							v.Functions[methodQName] = f
							qname = methodQName // Update for body visitor
						}
					}
				}
			} else {
				// Just a regular function
				f := extract.Function{
					Name:        fnObj.Name(),
					QName:       qname,
					ParentQName: "",
					Namespace:   namespace,
					Pos:         pos,
					End:         end,
					Filepath:    filepath,
					Code:        mCode,
				}

				v.extractParamsAndReturns(n, &f)
				v.extractDoc(n, &f)

				v.Functions[qname] = f
			}

			bv := &BodyVisitor{
				CallerQName: qname,
				Fset:        v.Fset,
				Info:        v.Info,
			}
			if n.Body != nil {
				ast.Walk(bv, n.Body)
			}
			f := v.Functions[qname]
			f.Calls = bv.Calls
			v.Functions[qname] = f

			return v
		}

	default:
		return v
	}
}

func (v *FunctionVisitor) extractDoc(n *ast.FuncDecl, f *extract.Function) {
	if n.Doc == nil {
		return
	}
	d := extract.Doc{
		Comment: n.Doc.Text(),
		OfQName: f.QName,
	}
	if len(n.Doc.List) == 1 {
		d.Type = extract.SingleLine
	} else if strings.HasPrefix(n.Doc.Text(), "/*") && strings.HasSuffix(n.Doc.Text(), "*/") {
		d.Type = extract.Block
	} else {
		d.Type = extract.MultiLine
	}

	f.Doc = d
}

func (v *FunctionVisitor) extractParamsAndReturns(n *ast.FuncDecl, f *extract.Function) {
	if n.Type.Params != nil {
		params := n.Type.Params.List
		for _, p := range params {
			for _, name := range p.Names {
				pObj := v.Info.Defs[name]
				f.ParamQNames = append(f.ParamQNames, pObj.Type().String())
			}
		}
	}

	if n.Type.Results != nil {
		results := n.Type.Results.List
		for _, r := range results {
			a, ok := v.Info.Types[r.Type]
			if !ok {
				continue
			}
			f.ReturnQNames = append(f.ReturnQNames, a.Type.String())
		}
	}

	// Build human-readable signature (always, even for functions with no params/returns)
	f.Signature = v.buildSignature(n, f.Name)
}

// buildSignature creates a human-readable signature from the AST.
// Examples:
//   - Function: "NewPlanner(cfg Config, arango Client) *Planner"
//   - Method: "(p *Planner) Plan(ctx context.Context, issue Issue) ([]Action, error)"
func (v *FunctionVisitor) buildSignature(n *ast.FuncDecl, name string) string {
	var sb strings.Builder

	// Receiver (for methods)
	if n.Recv != nil && len(n.Recv.List) > 0 {
		recv := n.Recv.List[0]
		sb.WriteString("(")
		if len(recv.Names) > 0 {
			sb.WriteString(recv.Names[0].Name)
			sb.WriteString(" ")
		}
		sb.WriteString(v.typeString(recv.Type))
		sb.WriteString(") ")
	}

	// Function name
	sb.WriteString(name)

	// Parameters
	sb.WriteString("(")
	if n.Type.Params != nil {
		v.writeFieldList(&sb, n.Type.Params.List)
	}
	sb.WriteString(")")

	// Return types
	if n.Type.Results != nil && len(n.Type.Results.List) > 0 {
		sb.WriteString(" ")
		if len(n.Type.Results.List) == 1 && len(n.Type.Results.List[0].Names) == 0 {
			// Single unnamed return
			sb.WriteString(v.typeString(n.Type.Results.List[0].Type))
		} else {
			// Multiple returns or named returns
			sb.WriteString("(")
			v.writeFieldList(&sb, n.Type.Results.List)
			sb.WriteString(")")
		}
	}

	return sb.String()
}

// writeFieldList writes parameters or return values to the builder.
func (v *FunctionVisitor) writeFieldList(sb *strings.Builder, fields []*ast.Field) {
	for i, field := range fields {
		if i > 0 {
			sb.WriteString(", ")
		}
		// Write names if present
		for j, name := range field.Names {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(name.Name)
		}
		if len(field.Names) > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(v.typeString(field.Type))
	}
}

// typeString converts an AST type expression to a readable string.
func (v *FunctionVisitor) typeString(expr ast.Expr) string {
	// Use the type info if available for accurate representation
	if t, ok := v.Info.Types[expr]; ok {
		return formatType(t.Type.String())
	}
	// Fallback to AST-based extraction
	return v.astTypeString(expr)
}

// astTypeString extracts type as string from AST when type info is unavailable.
func (v *FunctionVisitor) astTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + v.astTypeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + v.astTypeString(t.Elt)
		}
		return "[...]" + v.astTypeString(t.Elt)
	case *ast.SelectorExpr:
		return v.astTypeString(t.X) + "." + t.Sel.Name
	case *ast.MapType:
		return "map[" + v.astTypeString(t.Key) + "]" + v.astTypeString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + v.astTypeString(t.Value)
	case *ast.Ellipsis:
		return "..." + v.astTypeString(t.Elt)
	default:
		return "?"
	}
}

// formatType simplifies fully qualified type names for readability.
// e.g., "basegraph.app/relay/internal/model.Issue" -> "model.Issue"
func formatType(t string) string {
	// Handle pointer types
	if strings.HasPrefix(t, "*") {
		return "*" + formatType(t[1:])
	}
	// Handle slice types
	if strings.HasPrefix(t, "[]") {
		return "[]" + formatType(t[2:])
	}
	// Handle map types - find the last ] and format key and value
	if strings.HasPrefix(t, "map[") {
		// This is complex, just return as-is for maps
		return t
	}
	// For qualified names, keep only the last package.Type portion
	if idx := strings.LastIndex(t, "/"); idx != -1 {
		t = t[idx+1:]
	}
	return t
}
