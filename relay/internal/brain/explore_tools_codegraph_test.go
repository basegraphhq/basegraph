package brain_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/common/arangodb"
	"basegraph.app/relay/internal/brain"
)

type fakeArangoClient struct {
	searchSymbolsFn func(ctx context.Context, opts arangodb.SearchOptions) ([]arangodb.SearchResult, int, error)
	resolveSymbolFn func(ctx context.Context, opts arangodb.SearchOptions) (arangodb.ResolvedSymbol, error)
	fileSymbolsFn   func(ctx context.Context, opts arangodb.FileSymbolsOptions) ([]arangodb.FileSymbol, error)
	getCallersFn    func(ctx context.Context, qname string, depth int) ([]arangodb.GraphNode, error)
	getCalleesFn    func(ctx context.Context, qname string, depth int) ([]arangodb.GraphNode, error)
	getImplsFn      func(ctx context.Context, qname string) ([]arangodb.GraphNode, error)
	getUsagesFn     func(ctx context.Context, qname string) ([]arangodb.GraphNode, error)
	findCallPathFn  func(ctx context.Context, fromQName string, toQName string, maxDepth int) ([]arangodb.GraphNode, error)
	getChildrenFn   func(ctx context.Context, qname string) ([]arangodb.GraphNode, error)
	getMethodsFn    func(ctx context.Context, qname string) ([]arangodb.GraphNode, error)
	getInheritorsFn func(ctx context.Context, qname string) ([]arangodb.GraphNode, error)
	traverseFromFn  func(ctx context.Context, qnames []string, opts arangodb.TraversalOptions) ([]arangodb.GraphNode, []arangodb.GraphEdge, error)
	closeFn         func() error
}

func (f *fakeArangoClient) EnsureDatabase(ctx context.Context) error    { return nil }
func (f *fakeArangoClient) EnsureCollections(ctx context.Context) error { return nil }
func (f *fakeArangoClient) EnsureGraph(ctx context.Context) error       { return nil }
func (f *fakeArangoClient) IngestNodes(ctx context.Context, collection string, nodes []arangodb.Node) error {
	return nil
}

func (f *fakeArangoClient) IngestEdges(ctx context.Context, collection string, edges []arangodb.Edge) error {
	return nil
}
func (f *fakeArangoClient) TruncateCollections(ctx context.Context) error { return nil }

func (f *fakeArangoClient) GetCallers(ctx context.Context, qname string, depth int) ([]arangodb.GraphNode, error) {
	if f.getCallersFn != nil {
		return f.getCallersFn(ctx, qname, depth)
	}
	return nil, nil
}

func (f *fakeArangoClient) GetCallees(ctx context.Context, qname string, depth int) ([]arangodb.GraphNode, error) {
	if f.getCalleesFn != nil {
		return f.getCalleesFn(ctx, qname, depth)
	}
	return nil, nil
}

func (f *fakeArangoClient) FindCallPath(ctx context.Context, fromQName string, toQName string, maxDepth int) ([]arangodb.GraphNode, error) {
	if f.findCallPathFn != nil {
		return f.findCallPathFn(ctx, fromQName, toQName, maxDepth)
	}
	return nil, nil
}

func (f *fakeArangoClient) GetChildren(ctx context.Context, qname string) ([]arangodb.GraphNode, error) {
	if f.getChildrenFn != nil {
		return f.getChildrenFn(ctx, qname)
	}
	return nil, nil
}

func (f *fakeArangoClient) GetImplementations(ctx context.Context, qname string) ([]arangodb.GraphNode, error) {
	if f.getImplsFn != nil {
		return f.getImplsFn(ctx, qname)
	}
	return nil, nil
}

func (f *fakeArangoClient) GetMethods(ctx context.Context, qname string) ([]arangodb.GraphNode, error) {
	if f.getMethodsFn != nil {
		return f.getMethodsFn(ctx, qname)
	}
	return nil, nil
}

func (f *fakeArangoClient) GetUsages(ctx context.Context, qname string) ([]arangodb.GraphNode, error) {
	if f.getUsagesFn != nil {
		return f.getUsagesFn(ctx, qname)
	}
	return nil, nil
}

func (f *fakeArangoClient) GetInheritors(ctx context.Context, qname string) ([]arangodb.GraphNode, error) {
	if f.getInheritorsFn != nil {
		return f.getInheritorsFn(ctx, qname)
	}
	return nil, nil
}

func (f *fakeArangoClient) TraverseFrom(ctx context.Context, qnames []string, opts arangodb.TraversalOptions) ([]arangodb.GraphNode, []arangodb.GraphEdge, error) {
	if f.traverseFromFn != nil {
		return f.traverseFromFn(ctx, qnames, opts)
	}
	return nil, nil, nil
}

func (f *fakeArangoClient) GetFileSymbols(ctx context.Context, opts arangodb.FileSymbolsOptions) ([]arangodb.FileSymbol, error) {
	if f.fileSymbolsFn != nil {
		return f.fileSymbolsFn(ctx, opts)
	}
	return nil, nil
}

func (f *fakeArangoClient) SearchSymbols(ctx context.Context, opts arangodb.SearchOptions) ([]arangodb.SearchResult, int, error) {
	if f.searchSymbolsFn != nil {
		return f.searchSymbolsFn(ctx, opts)
	}
	return nil, 0, nil
}

func (f *fakeArangoClient) ResolveSymbol(ctx context.Context, opts arangodb.SearchOptions) (arangodb.ResolvedSymbol, error) {
	if f.resolveSymbolFn != nil {
		return f.resolveSymbolFn(ctx, opts)
	}
	return arangodb.ResolvedSymbol{}, arangodb.ErrNotFound
}

func (f *fakeArangoClient) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

var _ = Describe("ExploreTools codegraph", func() {
	var (
		ctx     context.Context
		tempDir string
		tools   *brain.ExploreTools
		fake    *fakeArangoClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "explore-tools-codegraph-test-*")
		Expect(err).NotTo(HaveOccurred())

		Expect(os.MkdirAll(filepath.Join(tempDir, "src"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, "src", "main.go"), []byte("package main\n\nfunc Plan() {}\n"), 0o644)).To(Succeed())

		fake = &fakeArangoClient{}
		tools = brain.NewExploreTools(tempDir, fake)
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("errors on invalid kind with supported kinds listed", func() {
		args, _ := json.Marshal(map[string]any{
			"operation": "search",
			"name":      "ActionExecutor",
			"kind":      "type",
		})

		result, err := tools.Execute(ctx, "codegraph", string(args))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("Error: invalid kind"))
		Expect(result).To(ContainSubstring("Supported kinds: function, method, struct, interface, class"))
	})

	It("auto-resolves name for callers (method->function fallback)", func() {
		fake.resolveSymbolFn = func(ctx context.Context, opts arangodb.SearchOptions) (arangodb.ResolvedSymbol, error) {
			if opts.Kind == "method" {
				return arangodb.ResolvedSymbol{}, arangodb.ErrNotFound
			}
			return arangodb.ResolvedSymbol{
				QName:     "example.com/app.Plan",
				Name:      "Plan",
				Kind:      "function",
				Filepath:  filepath.Join(tempDir, "src", "main.go"),
				Pos:       3,
				Signature: "func Plan() {}",
			}, nil
		}

		var calledQName string
		fake.getCallersFn = func(ctx context.Context, qname string, depth int) ([]arangodb.GraphNode, error) {
			calledQName = qname
			return []arangodb.GraphNode{
				{
					QName:     "example.com/app.Caller",
					Name:      "Caller",
					Kind:      "function",
					Filepath:  filepath.Join(tempDir, "src", "main.go"),
					Pos:       3,
					Signature: "func Plan() {}",
				},
			}, nil
		}

		args, _ := json.Marshal(map[string]any{
			"operation": "callers",
			"name":      "Plan",
			"depth":     2,
		})

		result, err := tools.Execute(ctx, "codegraph", string(args))
		Expect(err).NotTo(HaveOccurred())
		Expect(calledQName).To(Equal("example.com/app.Plan"))
		Expect(result).To(ContainSubstring("Callers of example.com/app.Plan"))
		Expect(result).To(ContainSubstring("src/main.go:3\tfunction\texample.com/app.Caller"))
	})

	It("formats ambiguous resolve with candidates", func() {
		fake.resolveSymbolFn = func(ctx context.Context, opts arangodb.SearchOptions) (arangodb.ResolvedSymbol, error) {
			return arangodb.ResolvedSymbol{}, arangodb.AmbiguousSymbolError{Query: opts.Name, Candidates: []arangodb.SearchResult{
				{QName: "example.com/a.Plan", Kind: "function", Filepath: filepath.Join(tempDir, "src", "main.go"), Pos: 3, Signature: "func Plan() {}"},
				{QName: "example.com/b.Plan", Kind: "function", Filepath: filepath.Join(tempDir, "src", "main.go"), Pos: 3, Signature: "func Plan() {}"},
			}}
		}

		args, _ := json.Marshal(map[string]any{
			"operation": "resolve",
			"name":      "Plan",
		})

		result, err := tools.Execute(ctx, "codegraph", string(args))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("Error: ambiguous symbol"))
		Expect(result).To(ContainSubstring("src/main.go:3\tfunction\texample.com/a.Plan"))
	})

	It("formats search results with file:line", func() {
		fake.searchSymbolsFn = func(ctx context.Context, opts arangodb.SearchOptions) ([]arangodb.SearchResult, int, error) {
			return []arangodb.SearchResult{{
				QName:     "example.com/app.Plan",
				Name:      "Plan",
				Kind:      "function",
				Filepath:  filepath.Join(tempDir, "src", "main.go"),
				Pos:       3,
				Signature: "func Plan() {}",
			}}, 1, nil
		}

		args, _ := json.Marshal(map[string]any{
			"operation": "search",
			"name":      "Plan",
			"kind":      "function",
		})

		result, err := tools.Execute(ctx, "codegraph", string(args))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("src/main.go:3\tfunction\texample.com/app.Plan"))
	})

	It("formats trace path", func() {
		fake.findCallPathFn = func(ctx context.Context, fromQName string, toQName string, maxDepth int) ([]arangodb.GraphNode, error) {
			return []arangodb.GraphNode{
				{QName: fromQName, Kind: "function", Filepath: filepath.Join(tempDir, "src", "main.go"), Pos: 3, Signature: "func A() {}"},
				{QName: toQName, Kind: "function", Filepath: filepath.Join(tempDir, "src", "main.go"), Pos: 3, Signature: "func B() {}"},
			}, nil
		}

		args, _ := json.Marshal(map[string]any{
			"operation":  "trace",
			"from_qname": "example.com/app.A",
			"to_qname":   "example.com/app.B",
			"max_depth":  6,
		})

		result, err := tools.Execute(ctx, "codegraph", string(args))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("Trace path from example.com/app.A to example.com/app.B"))
		Expect(result).To(ContainSubstring("src/main.go:3\tfunction\texample.com/app.A"))
		Expect(result).To(ContainSubstring("src/main.go:3\tfunction\texample.com/app.B"))
	})
})
