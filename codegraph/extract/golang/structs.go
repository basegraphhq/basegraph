package golang

import (
	"go/ast"
	"go/token"
	"go/types"
	"log/slog"
	"strings"

	extract "github.com/humanbeeng/lepo/prototypes/codegraph/extract"
)

type TypeVisitor struct {
	ast.Visitor
	Fset       *token.FileSet
	Info       *types.Info
	TypeDecls  map[string]extract.TypeDecl
	Implements map[string][]string
	Members    map[string]extract.Member
	Package    string
	TypeObjs   map[string]types.Type
}

func (v *TypeVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	switch nd := node.(type) {

	case *ast.GenDecl:
		for _, spec := range nd.Specs {
			tSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			tspecObj := v.Info.Defs[tSpec.Name]
			if tspecObj == nil {
				continue
			}

			if st, ok := tSpec.Type.(*ast.StructType); ok {
				namespace := extract.Namespace{Name: tspecObj.Pkg().Path()}
				stQName := namespace.Name + "." + tSpec.Name.Name
				pos := v.Fset.Position(st.Pos()).Line
				end := v.Fset.Position(st.End()).Line
				filepath := v.Fset.Position(st.Pos()).Filename

				stCode, err := extractCode(nd, v.Fset)
				if err != nil {
					// TODO: Handle errors gracefully
					panic(err)
				}
				impl, ok := v.Implements[stQName]
				if !ok {
					// Try with pointer type
					stQNameWPtr := "*" + stQName
					impl = v.Implements[stQNameWPtr]
				}

				td := extract.TypeDecl{
					Name:            tSpec.Name.Name,
					QName:           stQName,
					Namespace:       namespace,
					TypeQName:       tspecObj.Type().String(),
					Underlying:      tspecObj.Type().Underlying().String(),
					ImplementsQName: impl,
					Kind:            extract.Struct,
					Pos:             pos,
					End:             end,
					Filepath:        filepath,
					Code:            stCode,
					Doc: extract.Doc{
						Comment: nd.Doc.Text(),
						OfQName: stQName,
					},
				}

				v.TypeDecls[stQName] = td
				if v.TypeObjs != nil {
					v.TypeObjs[stQName] = tspecObj.Type()
				}

				fields := st.Fields
				for _, field := range fields.List {
					err := v.handleFieldNode(field, stQName)
					// TODO: Revisit on how to handle errors
					if err != nil {
						slog.Error("Unable to visit field", "err", err)
					}
				}
				continue
			}

			if id, ok := tSpec.Type.(*ast.Ident); ok {
				v.handleNonStructTypeSpec(nd, tSpec, id.Pos(), id.End())
				continue
			}

			v.handleNonStructTypeSpec(nd, tSpec, tSpec.Pos(), tSpec.End())
		}
		return v

	default:
		return v
	}
}

func (v *TypeVisitor) handleFieldNode(field *ast.Field, parentQName string) error {
	if field == nil {
		return nil
	}

	for _, fieldName := range field.Names {
		fieldObj := v.Info.Defs[fieldName]
		fieldQName := parentQName + "." + fieldObj.Name()
		namespace := extract.Namespace{Name: fieldObj.Pkg().Path()}

		d := extract.Doc{
			Comment: field.Doc.Text() + field.Comment.Text(),
			OfQName: fieldQName,
			// TODO: Add doc type
		}

		if field.Tag != nil {
			d.Comment = d.Comment + field.Tag.Value
		}
		st, ok := field.Type.(*ast.StructType)
		if ok && (strings.HasPrefix(fieldObj.Type().String(), "struct")) {
			pos := v.Fset.Position(field.Pos())
			end := v.Fset.Position(field.End())

			var stCode string
			stCode, err := extractCode(st, v.Fset)
			if err != nil {
				return err
			}

			ftd := extract.TypeDecl{
				Name:       fieldObj.Name(),
				QName:      fieldQName,
				Namespace:  namespace,
				TypeQName:  fieldObj.Type().String(),
				Underlying: fieldObj.Type().Underlying().String(),
				Kind:       extract.Struct,
				Code:       stCode,
				Pos:        pos.Line,
				Doc:        d,
				End:        end.Line,
				Filepath:   pos.Filename,
			}

			v.TypeDecls[fieldQName] = ftd

			fields := st.Fields
			for _, stf := range fields.List {
				err := v.handleFieldNode(stf, fieldQName)
				if err != nil {
					return err
				}
			}

		}
		m := extract.Member{
			Name:        fieldName.Name,
			QName:       fieldQName,
			TypeQName:   fieldObj.Type().String(),
			Namespace:   namespace,
			ParentQName: parentQName,
			Pos:         v.Fset.Position(field.Pos()).Line,
			End:         v.Fset.Position(field.End()).Line,
			Filepath:    v.Fset.Position(field.Pos()).Filename,
			// TODO: Extract member code
			Code: "",
			Doc:  d,
		}
		v.Members[fieldQName] = m
	}
	return nil
}

func (v *TypeVisitor) handleNonStructTypeSpec(nd *ast.GenDecl, tSpec *ast.TypeSpec, pos token.Pos, end token.Pos) {
	if tSpec == nil {
		return
	}

	tspecObj := v.Info.Defs[tSpec.Name]
	if tspecObj == nil {
		return
	}

	namespace := extract.Namespace{Name: tspecObj.Pkg().Path()}
	qname := namespace.Name + "." + tspecObj.Name()

	posInfo := v.Fset.Position(pos)
	endInfo := v.Fset.Position(end)

	impl, ok := v.Implements[qname]
	if !ok {
		qnameWPtr := "*" + qname
		impl = v.Implements[qnameWPtr]
	}

	doc := extract.Doc{
		Comment: nd.Doc.Text(),
		OfQName: qname,
	}

	kind := extract.Alias
	if typeName, ok := tspecObj.(*types.TypeName); ok {
		if !typeName.IsAlias() {
			if _, isStruct := tspecObj.Type().Underlying().(*types.Struct); isStruct {
				kind = extract.Struct
			}
		}
	}

	code, err := extractCode(tSpec, v.Fset)
	if err != nil {
		slog.Error("Unable to extract code for type spec", "qname", qname, "err", err)
		code = ""
	}

	td := extract.TypeDecl{
		Name:            tspecObj.Name(),
		QName:           qname,
		Namespace:       namespace,
		TypeQName:       tspecObj.Type().String(),
		Underlying:      tspecObj.Type().Underlying().String(),
		ImplementsQName: impl,
		Code:            code,
		Doc:             doc,
		Kind:            kind,
		Pos:             posInfo.Line,
		End:             endInfo.Line,
		Filepath:        posInfo.Filename,
	}

	v.TypeDecls[qname] = td
	if v.TypeObjs != nil {
		v.TypeObjs[qname] = tspecObj.Type()
	}
}
