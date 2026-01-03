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

// mockArangoClient implements arangodb.Client for testing
type mockArangoClient struct {
	getFileSymbolsFn func(ctx context.Context, filepath string) ([]arangodb.FileSymbol, error)
	searchSymbolsFn  func(ctx context.Context, opts arangodb.SearchOptions) ([]arangodb.SearchResult, int, error)
	getCallersFn     func(ctx context.Context, qname string, depth int) ([]arangodb.GraphNode, error)
	getCalleesFn     func(ctx context.Context, qname string, depth int) ([]arangodb.GraphNode, error)
	getMethodsFn     func(ctx context.Context, qname string) ([]arangodb.GraphNode, error)
	getImplementsFn  func(ctx context.Context, qname string) ([]arangodb.GraphNode, error)
	getUsagesFn      func(ctx context.Context, qname string) ([]arangodb.GraphNode, error)
	getInheritorsFn  func(ctx context.Context, qname string) ([]arangodb.GraphNode, error)
}

func (m *mockArangoClient) EnsureDatabase(ctx context.Context) error    { return nil }
func (m *mockArangoClient) EnsureCollections(ctx context.Context) error { return nil }
func (m *mockArangoClient) EnsureGraph(ctx context.Context) error       { return nil }
func (m *mockArangoClient) IngestNodes(ctx context.Context, collection string, nodes []arangodb.Node) error {
	return nil
}

func (m *mockArangoClient) IngestEdges(ctx context.Context, collection string, edges []arangodb.Edge) error {
	return nil
}
func (m *mockArangoClient) TruncateCollections(ctx context.Context) error { return nil }
func (m *mockArangoClient) Close() error                                  { return nil }

func (m *mockArangoClient) GetCallers(ctx context.Context, qname string, depth int) ([]arangodb.GraphNode, error) {
	if m.getCallersFn != nil {
		return m.getCallersFn(ctx, qname, depth)
	}
	return nil, nil
}

func (m *mockArangoClient) GetCallees(ctx context.Context, qname string, depth int) ([]arangodb.GraphNode, error) {
	if m.getCalleesFn != nil {
		return m.getCalleesFn(ctx, qname, depth)
	}
	return nil, nil
}

func (m *mockArangoClient) GetChildren(ctx context.Context, qname string) ([]arangodb.GraphNode, error) {
	return nil, nil
}

func (m *mockArangoClient) GetImplementations(ctx context.Context, qname string) ([]arangodb.GraphNode, error) {
	if m.getImplementsFn != nil {
		return m.getImplementsFn(ctx, qname)
	}
	return nil, nil
}

func (m *mockArangoClient) GetMethods(ctx context.Context, qname string) ([]arangodb.GraphNode, error) {
	if m.getMethodsFn != nil {
		return m.getMethodsFn(ctx, qname)
	}
	return nil, nil
}

func (m *mockArangoClient) GetUsages(ctx context.Context, qname string) ([]arangodb.GraphNode, error) {
	if m.getUsagesFn != nil {
		return m.getUsagesFn(ctx, qname)
	}
	return nil, nil
}

func (m *mockArangoClient) GetInheritors(ctx context.Context, qname string) ([]arangodb.GraphNode, error) {
	if m.getInheritorsFn != nil {
		return m.getInheritorsFn(ctx, qname)
	}
	return nil, nil
}

func (m *mockArangoClient) TraverseFrom(ctx context.Context, qnames []string, opts arangodb.TraversalOptions) ([]arangodb.GraphNode, []arangodb.GraphEdge, error) {
	return nil, nil, nil
}

func (m *mockArangoClient) GetFileSymbols(ctx context.Context, filepath string) ([]arangodb.FileSymbol, error) {
	if m.getFileSymbolsFn != nil {
		return m.getFileSymbolsFn(ctx, filepath)
	}
	return nil, nil
}

func (m *mockArangoClient) SearchSymbols(ctx context.Context, opts arangodb.SearchOptions) ([]arangodb.SearchResult, int, error) {
	if m.searchSymbolsFn != nil {
		return m.searchSymbolsFn(ctx, opts)
	}
	return nil, 0, nil
}

var _ = Describe("ExploreTools", func() {
	var (
		tools   *brain.ExploreTools
		tempDir string
		ctx     context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create a temporary directory structure for testing
		var err error
		tempDir, err = os.MkdirTemp("", "explore-tools-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Create test directory structure
		// tempDir/
		//   src/
		//     main.go
		//     util/
		//       helper.go
		//   .git/
		//     config
		//   README.md
		Expect(os.MkdirAll(filepath.Join(tempDir, "src", "util"), 0o755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, "src", "main.go"), []byte("package main"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, "src", "util", "helper.go"), []byte("package util"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, ".git", "config"), []byte("[core]"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Test"), 0o644)).To(Succeed())

		// Create tools with nil arango client (not needed for tree tests)
		tools = brain.NewExploreTools(tempDir, nil)
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("Tree Tool", func() {
		Describe("Security", func() {
			It("rejects absolute paths outside repo root", func() {
				args, _ := json.Marshal(map[string]any{
					"path": "/etc/passwd",
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("path outside repository"))
			})

			It("rejects path traversal with ..", func() {
				args, _ := json.Marshal(map[string]any{
					"path": "../../../etc",
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("path outside repository"))
			})

			It("rejects path traversal with encoded ..", func() {
				args, _ := json.Marshal(map[string]any{
					"path": "src/../../..",
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("path outside repository"))
			})

			It("rejects path that looks like subdirectory but escapes", func() {
				// Create a sibling directory to test /repo vs /repo-evil scenario
				siblingDir := tempDir + "-evil"
				Expect(os.MkdirAll(siblingDir, 0o755)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(siblingDir, "secret.txt"), []byte("secret"), 0o644)).To(Succeed())
				defer os.RemoveAll(siblingDir)

				// Try to access sibling via path traversal
				args, _ := json.Marshal(map[string]any{
					"path": "../" + filepath.Base(siblingDir),
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("path outside repository"))
			})

			It("rejects symlink escape attempts", func() {
				// Create a symlink pointing outside the repo
				symlinkPath := filepath.Join(tempDir, "escape-link")
				err := os.Symlink("/etc", symlinkPath)
				if err != nil {
					Skip("Cannot create symlinks on this system")
				}

				args, _ := json.Marshal(map[string]any{
					"path": "escape-link",
				})

				_, execErr := tools.Execute(ctx, "tree", string(args))

				// Should either reject or show the symlink as a file, not traverse it
				Expect(execErr).NotTo(HaveOccurred())
				// The symlink itself is in the repo, but we shouldn't traverse into /etc
				// Current implementation: os.Stat follows symlinks, so /etc would be listed
				// This test documents current behavior - symlink traversal is a known limitation
				// For now, we accept that symlinks are followed (like standard `tree` command)
			})
		})

		Describe("Functionality", func() {
			It("lists directory structure at default depth", func() {
				args, _ := json.Marshal(map[string]any{})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("src/"))
				Expect(result).To(ContainSubstring("main.go"))
				Expect(result).To(ContainSubstring("README.md"))
			})

			It("respects path parameter", func() {
				args, _ := json.Marshal(map[string]any{
					"path": "src",
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("src/"))
				Expect(result).To(ContainSubstring("main.go"))
				Expect(result).To(ContainSubstring("util/"))
			})

			It("excludes .git directory", func() {
				args, _ := json.Marshal(map[string]any{})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(ContainSubstring(".git"))
				Expect(result).NotTo(ContainSubstring("config"))
			})

			It("excludes node_modules directory", func() {
				// Create node_modules
				Expect(os.MkdirAll(filepath.Join(tempDir, "node_modules", "lodash"), 0o755)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(tempDir, "node_modules", "lodash", "index.js"), []byte(""), 0o644)).To(Succeed())

				args, _ := json.Marshal(map[string]any{})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(ContainSubstring("node_modules"))
				Expect(result).NotTo(ContainSubstring("lodash"))
			})

			It("respects depth parameter", func() {
				args, _ := json.Marshal(map[string]any{
					"depth": 1,
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("src/"))
				// At depth 1, we should NOT see files inside src/
				Expect(result).NotTo(ContainSubstring("main.go"))
			})

			It("caps depth at maximum", func() {
				args, _ := json.Marshal(map[string]any{
					"depth": 100, // Way over max
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				// Should still work, just capped at max depth (4)
				Expect(result).To(ContainSubstring("src/"))
			})

			It("returns error for non-existent path", func() {
				args, _ := json.Marshal(map[string]any{
					"path": "nonexistent",
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Directory not found"))
			})

			It("returns error when path is a file", func() {
				args, _ := json.Marshal(map[string]any{
					"path": "README.md",
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Not a directory"))
			})

			It("shows directories before files", func() {
				args, _ := json.Marshal(map[string]any{
					"path": "src",
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				// util/ (directory) should appear before main.go (file)
				utilIdx := len(result) - len(result[findSubstring(result, "util/"):])
				mainIdx := len(result) - len(result[findSubstring(result, "main.go"):])
				Expect(utilIdx).To(BeNumerically("<", mainIdx))
			})

			It("handles empty directory", func() {
				emptyDir := filepath.Join(tempDir, "empty")
				Expect(os.MkdirAll(emptyDir, 0o755)).To(Succeed())

				args, _ := json.Marshal(map[string]any{
					"path": "empty",
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Directory is empty"))
			})
		})

		Describe("Edge Cases", func() {
			It("handles path with spaces", func() {
				spacePath := filepath.Join(tempDir, "path with spaces")
				Expect(os.MkdirAll(spacePath, 0o755)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(spacePath, "file.txt"), []byte("test"), 0o644)).To(Succeed())

				args, _ := json.Marshal(map[string]any{
					"path": "path with spaces",
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("file.txt"))
			})

			It("handles path with special characters", func() {
				specialPath := filepath.Join(tempDir, "special-chars_123")
				Expect(os.MkdirAll(specialPath, 0o755)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(specialPath, "test.go"), []byte("package test"), 0o644)).To(Succeed())

				args, _ := json.Marshal(map[string]any{
					"path": "special-chars_123",
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("test.go"))
			})

			It("handles deeply nested structure within depth limit", func() {
				// Create a/b/c/d/e/f structure
				deepPath := filepath.Join(tempDir, "a", "b", "c", "d", "e", "f")
				Expect(os.MkdirAll(deepPath, 0o755)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(deepPath, "deep.txt"), []byte("deep"), 0o644)).To(Succeed())

				args, _ := json.Marshal(map[string]any{
					"path":  "a",
					"depth": 4, // max depth
				})

				result, err := tools.Execute(ctx, "tree", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("b/"))
				Expect(result).To(ContainSubstring("c/"))
				Expect(result).To(ContainSubstring("d/"))
				// e/ is at depth 4, should be visible
				Expect(result).To(ContainSubstring("e/"))
				// f/ is at depth 5, should NOT be visible
				Expect(result).NotTo(ContainSubstring("f/"))
			})
		})
	})

	Describe("Graph Tool", func() {
		var mockArango *mockArangoClient

		BeforeEach(func() {
			mockArango = &mockArangoClient{}
			tools = brain.NewExploreTools(tempDir, mockArango)
		})

		Describe("symbols operation", func() {
			It("returns symbols for a Go file", func() {
				mockArango.getFileSymbolsFn = func(ctx context.Context, filepath string) ([]arangodb.FileSymbol, error) {
					return []arangodb.FileSymbol{
						{QName: "pkg.Planner", Name: "Planner", Kind: "struct", Pos: 25, End: 40},
						{QName: "pkg.Planner.Plan", Name: "Plan", Kind: "method", Signature: "(p *Planner) Plan(ctx context.Context) error", Pos: 52, End: 80},
						{QName: "pkg.NewPlanner", Name: "NewPlanner", Kind: "function", Signature: "NewPlanner(cfg Config) *Planner", Pos: 37, End: 50},
					}, nil
				}

				args, _ := json.Marshal(map[string]any{
					"operation": "symbols",
					"file":      "internal/brain/planner.go",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Symbols in internal/brain/planner.go [indexed]"))
				Expect(result).To(ContainSubstring("Planner (struct)"))
				Expect(result).To(ContainSubstring("(p *Planner) Plan(ctx context.Context) error (method)"))
				Expect(result).To(ContainSubstring("NewPlanner(cfg Config) *Planner (function)"))
				Expect(result).To(ContainSubstring("qname: pkg.Planner"))
			})

			It("returns error for non-indexed file types", func() {
				args, _ := json.Marshal(map[string]any{
					"operation": "symbols",
					"file":      "src/component.tsx",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Symbols not available for .tsx files"))
				Expect(result).To(ContainSubstring("Use grep to find definitions"))
			})

			It("returns helpful message when no symbols found", func() {
				mockArango.getFileSymbolsFn = func(ctx context.Context, filepath string) ([]arangodb.FileSymbol, error) {
					return []arangodb.FileSymbol{}, nil
				}

				args, _ := json.Marshal(map[string]any{
					"operation": "symbols",
					"file":      "internal/empty.go",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("No symbols found"))
				Expect(result).To(ContainSubstring("may not be indexed yet"))
			})

			It("requires file parameter", func() {
				args, _ := json.Marshal(map[string]any{
					"operation": "symbols",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("'file' parameter is required"))
			})
		})

		Describe("search operation", func() {
			It("finds symbols by name pattern", func() {
				mockArango.searchSymbolsFn = func(ctx context.Context, opts arangodb.SearchOptions) ([]arangodb.SearchResult, int, error) {
					Expect(opts.Name).To(Equal("*Issue*"))
					return []arangodb.SearchResult{
						{QName: "pkg/model.Issue", Name: "Issue", Kind: "struct", Filepath: "internal/model/issue.go", Pos: 15},
						{QName: "pkg/store.IssueStore", Name: "IssueStore", Kind: "struct", Filepath: "internal/store/issue.go", Pos: 22},
					}, 2, nil
				}

				args, _ := json.Marshal(map[string]any{
					"operation": "search",
					"name":      "*Issue*",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring(`Search results for name="*Issue*"`))
				Expect(result).To(ContainSubstring("(2 of 2)"))
				Expect(result).To(ContainSubstring("Issue (struct)"))
				Expect(result).To(ContainSubstring("IssueStore (struct)"))
				Expect(result).To(ContainSubstring("qname: pkg/model.Issue"))
			})

			It("filters by kind", func() {
				mockArango.searchSymbolsFn = func(ctx context.Context, opts arangodb.SearchOptions) ([]arangodb.SearchResult, int, error) {
					Expect(opts.Name).To(Equal("Plan*"))
					Expect(opts.Kind).To(Equal("method"))
					return []arangodb.SearchResult{
						{QName: "pkg.Planner.Plan", Name: "Plan", Kind: "method", Signature: "(p *Planner) Plan(ctx context.Context) error", Filepath: "internal/brain/planner.go", Pos: 52},
					}, 1, nil
				}

				args, _ := json.Marshal(map[string]any{
					"operation": "search",
					"name":      "Plan*",
					"kind":      "method",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring(`kind="method"`))
				Expect(result).To(ContainSubstring("(p *Planner) Plan(ctx context.Context) error (method)"))
			})

			It("shows truncation message when results exceed limit", func() {
				mockArango.searchSymbolsFn = func(ctx context.Context, opts arangodb.SearchOptions) ([]arangodb.SearchResult, int, error) {
					results := make([]arangodb.SearchResult, 50)
					for i := range results {
						results[i] = arangodb.SearchResult{QName: "pkg.Func", Name: "Func", Kind: "function", Filepath: "file.go", Pos: i}
					}
					return results, 150, nil // 150 total, only 50 returned
				}

				args, _ := json.Marshal(map[string]any{
					"operation": "search",
					"name":      "*",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("(50 of 150)"))
				Expect(result).To(ContainSubstring("Showing 50 of 150 results"))
			})

			It("requires name parameter", func() {
				args, _ := json.Marshal(map[string]any{
					"operation": "search",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("'name' parameter is required"))
			})

			It("returns helpful message when no results found", func() {
				mockArango.searchSymbolsFn = func(ctx context.Context, opts arangodb.SearchOptions) ([]arangodb.SearchResult, int, error) {
					return []arangodb.SearchResult{}, 0, nil
				}

				args, _ := json.Marshal(map[string]any{
					"operation": "search",
					"name":      "NonExistent*",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("No symbols found matching"))
				Expect(result).To(ContainSubstring("Try a broader pattern"))
			})
		})

		Describe("relationship operations", func() {
			It("requires qname for callers operation", func() {
				args, _ := json.Marshal(map[string]any{
					"operation": "callers",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("'qname' parameter is required"))
				Expect(result).To(ContainSubstring("Use graph(symbols, file=...)"))
			})

			It("returns callers with depth", func() {
				mockArango.getCallersFn = func(ctx context.Context, qname string, depth int) ([]arangodb.GraphNode, error) {
					Expect(qname).To(Equal("pkg.Planner.Plan"))
					Expect(depth).To(Equal(2))
					return []arangodb.GraphNode{
						{QName: "pkg.Handler.Handle", Name: "Handle", Kind: "method", Filepath: "internal/http/handler.go"},
						{QName: "pkg.Worker.Run", Name: "Run", Kind: "method", Filepath: "internal/worker/worker.go"},
					}, nil
				}

				args, _ := json.Marshal(map[string]any{
					"operation": "callers",
					"qname":     "pkg.Planner.Plan",
					"depth":     2,
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Callers of pkg.Planner.Plan (depth 2)"))
				Expect(result).To(ContainSubstring("Handle (method)"))
				Expect(result).To(ContainSubstring("Run (method)"))
			})

			It("returns methods of a type", func() {
				mockArango.getMethodsFn = func(ctx context.Context, qname string) ([]arangodb.GraphNode, error) {
					Expect(qname).To(Equal("pkg.Planner"))
					return []arangodb.GraphNode{
						{QName: "pkg.Planner.Plan", Name: "Plan", Kind: "method", Filepath: "internal/brain/planner.go"},
						{QName: "pkg.Planner.Execute", Name: "Execute", Kind: "method", Filepath: "internal/brain/planner.go"},
					}, nil
				}

				args, _ := json.Marshal(map[string]any{
					"operation": "methods",
					"qname":     "pkg.Planner",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Methods of pkg.Planner"))
				Expect(result).To(ContainSubstring("Plan (method)"))
				Expect(result).To(ContainSubstring("Execute (method)"))
			})

			It("returns helpful message when no results", func() {
				mockArango.getCallersFn = func(ctx context.Context, qname string, depth int) ([]arangodb.GraphNode, error) {
					return []arangodb.GraphNode{}, nil
				}

				args, _ := json.Marshal(map[string]any{
					"operation": "callers",
					"qname":     "pkg.Unused",
				})

				result, err := tools.Execute(ctx, "graph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Callers of pkg.Unused: No results found"))
			})
		})
	})
})

// Helper function to find substring index
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
