package golang

import (
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/humanbeeng/lepo/prototypes/codegraph/extract"
)

func TestAugmentImplementsForNamedTypes(t *testing.T) {
	pkgPath := "example.com/logging"
	pkgName := "logging"
	pkg := types.NewPackage(pkgPath, pkgName)

	// Interfaces
	loggerIfaceQName := pkgPath + ".Logger"
	closerIfaceQName := pkgPath + ".Closer"
	emptyIfaceQName := pkgPath + ".Empty"

	logMethod := makeInterfaceMethod(pkg, "Log")
	loggerIface := types.NewInterfaceType([]*types.Func{logMethod}, nil)
	loggerIface.Complete()

	closeMethod := makeInterfaceMethod(pkg, "Close")
	closerIface := types.NewInterfaceType([]*types.Func{closeMethod}, nil)
	closerIface.Complete()

	emptyIface := types.NewInterfaceType(nil, nil)
	emptyIface.Complete()

	// Types
	valueNamed := makeNamedStruct(pkg, "FileLogger")
	valueNamed.AddMethod(makeMethod(pkg, valueNamed, "Log"))

	pointerNamed := makeNamedStruct(pkg, "BufferedLogger")
	pointerNamed.AddMethod(makeMethod(pkg, types.NewPointer(pointerNamed), "Close"))

	alreadyNamed := makeNamedStruct(pkg, "ExistingLogger")
	alreadyNamed.AddMethod(makeMethod(pkg, alreadyNamed, "Log"))

	noMethodNamed := makeNamedStruct(pkg, "MetricsCollector")

	valueQName := pkgPath + ".FileLogger"
	pointerQName := pkgPath + ".BufferedLogger"
	alreadyQName := pkgPath + ".ExistingLogger"
	noMethodQName := pkgPath + ".MetricsCollector"

	typeDecls := map[string]extract.TypeDecl{
		valueQName: {
			Name:            "FileLogger",
			QName:           valueQName,
			ImplementsQName: nil,
		},
		pointerQName: {
			Name:            "BufferedLogger",
			QName:           pointerQName,
			ImplementsQName: nil,
		},
		alreadyQName: {
			Name:            "ExistingLogger",
			QName:           alreadyQName,
			ImplementsQName: []string{loggerIfaceQName},
		},
		noMethodQName: {
			Name:            "MetricsCollector",
			QName:           noMethodQName,
			ImplementsQName: nil,
		},
	}

	typeObjs := map[string]types.Type{
		valueQName:    valueNamed,
		pointerQName:  pointerNamed,
		alreadyQName:  alreadyNamed,
		noMethodQName: noMethodNamed,
	}

	interfaceObjs := map[string]*types.Interface{
		loggerIfaceQName: loggerIface,
		closerIfaceQName: closerIface,
		emptyIfaceQName:  emptyIface,
	}

	augmentImplementsForNamedTypes(typeObjs, interfaceObjs, typeDecls)

	if !slices.Contains(typeDecls[valueQName].ImplementsQName, loggerIfaceQName) {
		t.Fatalf("expected %s to include %s, got %v", valueQName, loggerIfaceQName, typeDecls[valueQName].ImplementsQName)
	}

	if !slices.Contains(typeDecls[pointerQName].ImplementsQName, closerIfaceQName) {
		t.Fatalf("expected %s to include %s, got %v", pointerQName, closerIfaceQName, typeDecls[pointerQName].ImplementsQName)
	}

	if countOccurrences(typeDecls[alreadyQName].ImplementsQName, loggerIfaceQName) != 1 {
		t.Fatalf("expected %s to retain single %s entry, got %v", alreadyQName, loggerIfaceQName, typeDecls[alreadyQName].ImplementsQName)
	}

	if len(typeDecls[noMethodQName].ImplementsQName) != 0 {
		t.Fatalf("expected %s to remain empty, got %v", noMethodQName, typeDecls[noMethodQName].ImplementsQName)
	}

	if slices.Contains(typeDecls[valueQName].ImplementsQName, emptyIfaceQName) ||
		slices.Contains(typeDecls[pointerQName].ImplementsQName, emptyIfaceQName) {
		t.Fatalf("empty interface should not be recorded, got value=%v pointer=%v",
			typeDecls[valueQName].ImplementsQName,
			typeDecls[pointerQName].ImplementsQName,
		)
	}
}

func TestExtractorCapturesImplementsRelationships(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "go.mod"), `module example.com/testpkg

go 1.24
`)

	writeFile(t, filepath.Join(dir, "notification", "notification.go"), `package notification

type Notifier interface {
	Notify()
}

type EmailNotifier struct{}

func (EmailNotifier) Notify() {}

type SlackNotifier struct{}

func (*SlackNotifier) Notify() {}

type AliasEmailNotifier = EmailNotifier

type CustomEmailNotifier EmailNotifier

func (CustomEmailNotifier) Notify() {}

type NullNotifier struct{}

var (
	_ Notifier = EmailNotifier{}
	_ Notifier = &SlackNotifier{}
)
`)

	extractor := NewGoExtractor()
	res, err := extractor.Extract("example.com/testpkg/notification", dir)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	getTypeDecl := func(name string) extract.TypeDecl {
		qname := "example.com/testpkg/notification." + name
		td, ok := res.TypeDecls[qname]
		if !ok {
			t.Fatalf("missing type decl for %s", qname)
		}
		return td
	}

	expectImplements := func(td extract.TypeDecl, iface string) {
		t.Helper()
		t.Logf("Verifying %s implements %s", td.QName, iface)
		qname := "example.com/testpkg/notification." + iface
		if !slices.Contains(td.ImplementsQName, qname) {
			t.Fatalf("expected %s to implement %s, got %v", td.QName, qname, td.ImplementsQName)
		}
	}

	expectNoInterface := func(td extract.TypeDecl) {
		t.Helper()
		if len(td.ImplementsQName) != 0 {
			t.Fatalf("expected %s to have no implements entries, got %v", td.QName, td.ImplementsQName)
		}
	}

	ifaceName := "Notifier"

	emailType := getTypeDecl("EmailNotifier")
	expectImplements(emailType, ifaceName)

	slackType := getTypeDecl("SlackNotifier")
	expectImplements(slackType, ifaceName)

	aliasType := getTypeDecl("AliasEmailNotifier")
	expectImplements(aliasType, ifaceName)

	customType := getTypeDecl("CustomEmailNotifier")
	expectImplements(customType, ifaceName)

	noMethod := getTypeDecl("NullNotifier")
	expectNoInterface(noMethod)
}

func makeNamedStruct(pkg *types.Package, name string) *types.Named {
	obj := types.NewTypeName(token.NoPos, pkg, name, nil)
	return types.NewNamed(obj, types.NewStruct(nil, nil), nil)
}

func makeMethod(pkg *types.Package, recv types.Type, name string) *types.Func {
	recvVar := types.NewVar(token.NoPos, pkg, "", recv)
	sig := types.NewSignatureType(recvVar, nil, nil, types.NewTuple(), types.NewTuple(), false)
	return types.NewFunc(token.NoPos, pkg, name, sig)
}

func makeInterfaceMethod(pkg *types.Package, name string) *types.Func {
	sig := types.NewSignatureType(nil, nil, nil, types.NewTuple(), types.NewTuple(), false)
	return types.NewFunc(token.NoPos, pkg, name, sig)
}

func countOccurrences(values []string, target string) int {
	count := 0
	for _, v := range values {
		if v == target {
			count++
		}
	}
	return count
}

func writeFile(t *testing.T, filename, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatalf("mkdir failed for %s: %v", filename, err)
	}
	if err := os.WriteFile(filename, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file failed for %s: %v", filename, err)
	}
}
