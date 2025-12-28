package golang

import (
	"go/ast"
	"go/token"
	"go/types"
)

type BodyVisitor struct {
	ast.Visitor
	CallerQName string
	Calls       []string
	Fset        *token.FileSet
	Info        *types.Info
}

func (v *BodyVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	switch n := node.(type) {
	case *ast.CallExpr:
		{
			v.handleCallExpr(n)
			return v
		}
	default:
		return v
	}
}

func (v *BodyVisitor) handleCallExpr(ce *ast.CallExpr) {
	if ce == nil {
		return
	}
	if id, ok := ce.Fun.(*ast.Ident); ok {
		// Direct function call: funcName()
		ceObj := v.Info.Uses[id]
		if ceObj != nil {
			if ceObj.Pkg() == nil {
				return
			}
			callee := ceObj.Pkg().Path() + "." + ceObj.Name()
			v.Calls = append(v.Calls, callee)
		}
	} else if se, ok := ce.Fun.(*ast.SelectorExpr); ok {
		// Method or field call: receiver.Method()
		seObj := v.Info.Uses[se.Sel]
		if seObj == nil || seObj.Pkg() == nil {
			return
		}

		// Check if this is a method call (has receiver type)
		if fn, ok := seObj.(*types.Func); ok {
			sig := fn.Type().(*types.Signature)
			recv := sig.Recv()
			if recv != nil {
				// This is a method call - include type in qname
				recvType := recv.Type()
				// Handle pointer types
				if ptr, ok := recvType.(*types.Pointer); ok {
					recvType = ptr.Elem()
				}
				if named, ok := recvType.(*types.Named); ok {
					typeName := named.Obj().Name()
					callee := seObj.Pkg().Path() + "." + typeName + "." + seObj.Name()
					v.Calls = append(v.Calls, callee)
					return
				}
			}
		}

		// Regular function call on package
		callee := seObj.Pkg().Path() + "." + seObj.Name()
		v.Calls = append(v.Calls, callee)
	}
}
