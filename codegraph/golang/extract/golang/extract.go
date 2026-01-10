package golang

import (
	"go/ast"
	"go/token"
	"go/types"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/refactor/satisfy"

	"github.com/humanbeeng/lepo/prototypes/codegraph/extract"
)

type GoExtractor struct{}

func NewGoExtractor() *GoExtractor {
	return &GoExtractor{}
}

func (g *GoExtractor) Extract(pkgstr string, dir string) (extract.ExtractNodesResult, error) {
	// TODO: Change or add directory path as well.
	start := time.Now()

	slog.Info("Extraction requested for", "package", pkgstr)
	// same into cfg.Check method
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedName | packages.NeedTypesInfo | packages.NeedImports,
		Fset:  fset,
		Dir:   dir,
		Tests: true,
	}

	pattern := pkgstr
	switch {
	case pattern == "":
		pattern = "./..."
	case strings.HasSuffix(pattern, "/..."):
		// already recursive
	default:
		pattern = pattern + "/..."
	}

	// TODO: Take directory as input and get extract pkgstr using go mod file
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		slog.Error("Unable to load", "package", pkgstr)
		return extract.ExtractNodesResult{}, err
	}

	slog.Info("Packages found", "count", len(pkgs))

	implMap := make(map[string][]string)

	extractRes := extract.ExtractNodesResult{
		TypeDecls:  make(map[string]extract.TypeDecl),
		Interfaces: make(map[string]extract.TypeDecl),
		NamedTypes: make(map[string]extract.Named),
		Members:    make(map[string]extract.Member),
		Functions:  make(map[string]extract.Function),
		Files:      make(map[string]extract.File),
		Vars:       make(map[string]extract.Variable),
	}

	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		// Process implementations of given package only.
		if pkgstr != "" && !strings.HasPrefix(pkg.PkgPath, pkgstr) {
			return
		}

		// Skip packages with type errors (e.g., unresolved imports)
		if len(pkg.Errors) > 0 {
			slog.Warn("Skipping package with errors", "package", pkg.PkgPath, "errors", len(pkg.Errors))
			for _, e := range pkg.Errors {
				slog.Debug("Package error", "package", pkg.PkgPath, "error", e.Error())
			}
			return
		}

		slog.Info("Constructing implementors map", "package", pkg.PkgPath)

		fi := satisfy.Finder{Result: make(map[satisfy.Constraint]bool)}
		fi.Find(pkg.TypesInfo, pkg.Syntax)

		// Transform Finder Result map to make it queryable
		for r := range fi.Result {
			implMap[r.RHS.String()] = append(implMap[r.RHS.String()], r.LHS.String())
		}
	})

	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		// Process nodes of given package only.
		if pkgstr != "" && !strings.HasPrefix(pkg.PkgPath, pkgstr) {
			return
		}

		// Skip packages with type errors (e.g., unresolved imports)
		if len(pkg.Errors) > 0 {
			return
		}

		extractRes.Namespaces = append(
			extractRes.Namespaces,
			extract.Namespace{
				Name: pkg.PkgPath,
			},
		)

		slog.Info("Analysing", "package", pkg.PkgPath)
		slog.Info("Files found in", "package", pkg.PkgPath, "count", len(pkg.Syntax))

		typeObjs := make(map[string]types.Type)
		interfaceObjs := make(map[string]*types.Interface)

		tv := &TypeVisitor{
			Fset:       fset,
			Info:       pkg.TypesInfo,
			TypeDecls:  extractRes.TypeDecls,
			Implements: implMap,
			Members:    extractRes.Members,
			Package:    pkg.PkgPath,
			TypeObjs:   typeObjs,
		}

		nv := &NamedVisitor{
			Fset:  fset,
			Info:  pkg.TypesInfo,
			Named: extractRes.NamedTypes,
		}

		vv := &VarVisitor{
			Fset: fset,
			Info: pkg.TypesInfo,
			Vars: extractRes.Vars,
		}

		fv := &FunctionVisitor{
			Fset:      fset,
			Info:      pkg.TypesInfo,
			Functions: extractRes.Functions,
		}

		fiv := &FileVisitor{
			Package: pkg.PkgPath,
			Fset:    fset,
			Info:    pkg.TypesInfo,
			Files:   extractRes.Files,
		}

		iv := &InterfaceVisitor{
			Fset:          fset,
			Info:          pkg.TypesInfo,
			Interfaces:    extractRes.Interfaces,
			Members:       extractRes.Members,
			InterfaceObjs: interfaceObjs,
		}

		for _, file := range pkg.Syntax {
			slog.Info("Walking", "file", fset.Position(file.Pos()).Filename)
			ast.Walk(tv, file)
			ast.Walk(nv, file)
			ast.Walk(fv, file)
			ast.Walk(fiv, file)
			ast.Walk(iv, file)
			ast.Walk(vv, file)
		}

		augmentImplementsForNamedTypes(typeObjs, interfaceObjs, extractRes.TypeDecls)
	})

	slog.Info("Extraction completed", "time_taken", time.Since(start).String())

	return extractRes, nil
}

func augmentImplementsForNamedTypes(
	typeObjs map[string]types.Type,
	interfaceObjs map[string]*types.Interface,
	decls map[string]extract.TypeDecl,
) {
	if len(typeObjs) == 0 || len(interfaceObjs) == 0 {
		return
	}

	for iface := range interfaceObjs {
		if it := interfaceObjs[iface]; it != nil {
			it.Complete()
		}
	}

	for typeQName, typ := range typeObjs {
		td, ok := decls[typeQName]
		if !ok {
			continue
		}

		seen := make(map[string]struct{}, len(td.ImplementsQName))
		for _, existing := range td.ImplementsQName {
			seen[existing] = struct{}{}
		}

		updated := false
		for ifaceQName, iface := range interfaceObjs {
			if iface == nil || iface.NumMethods() == 0 {
				continue
			}
			if _, present := seen[ifaceQName]; present {
				continue
			}
			if typeSatisfiesInterface(typ, iface) {
				td.ImplementsQName = append(td.ImplementsQName, ifaceQName)
				seen[ifaceQName] = struct{}{}
				updated = true
			}
		}

		if updated {
			decls[typeQName] = td
		}
	}
}

func typeSatisfiesInterface(typ types.Type, iface *types.Interface) bool {
	if typ == nil || iface == nil {
		return false
	}

	if types.Implements(typ, iface) {
		return true
	}

	if named, ok := typ.(*types.Named); ok {
		if types.Implements(types.NewPointer(named), iface) {
			return true
		}
	}

	return false
}
