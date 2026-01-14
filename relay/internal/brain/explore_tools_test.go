package brain_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"basegraph.app/relay/internal/brain"
)

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
		//   README.md
		Expect(os.MkdirAll(filepath.Join(tempDir, "src", "util"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, "src", "main.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, "src", "util", "helper.go"), []byte("package util\n\nfunc Helper() string {\n\treturn \"help\"\n}\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Test Project\n\nThis is a test.\n"), 0o644)).To(Succeed())

		// Create tools for testing (nil arango client since we're only testing file tools)
		tools = brain.NewExploreTools(tempDir, nil)
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("Read Tool", func() {
		It("reads a file", func() {
			args, _ := json.Marshal(map[string]any{
				"file_path": "src/main.go",
			})

			result, err := tools.Execute(ctx, "read", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("package main"))
			Expect(result).To(ContainSubstring("func main()"))
		})

		It("reads with offset", func() {
			args, _ := json.Marshal(map[string]any{
				"file_path": "src/main.go",
				"offset":    3,
			})

			result, err := tools.Execute(ctx, "read", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("func main()"))
			Expect(result).NotTo(ContainSubstring("package main"))
		})

		It("reads with limit", func() {
			args, _ := json.Marshal(map[string]any{
				"file_path": "src/main.go",
				"limit":     2,
			})

			result, err := tools.Execute(ctx, "read", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("package main"))
			// Should indicate only 2 lines were read
			Expect(result).To(ContainSubstring("lines 1-2"))
		})

		It("returns error for missing file", func() {
			args, _ := json.Marshal(map[string]any{
				"file_path": "nonexistent.go",
			})

			result, err := tools.Execute(ctx, "read", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("file not found"))
		})

		It("returns error for path outside repo", func() {
			args, _ := json.Marshal(map[string]any{
				"file_path": "../../../etc/passwd",
			})

			result, err := tools.Execute(ctx, "read", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("outside repository"))
		})

		It("returns error for missing file_path", func() {
			args, _ := json.Marshal(map[string]any{})

			result, err := tools.Execute(ctx, "read", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("file_path is required"))
		})
	})

	Describe("Grep Tool", func() {
		It("finds pattern in files", func() {
			args, _ := json.Marshal(map[string]any{
				"pattern": "package",
			})

			result, err := tools.Execute(ctx, "grep", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("main.go"))
			Expect(result).To(ContainSubstring("helper.go"))
		})

		It("finds pattern with glob filter", func() {
			args, _ := json.Marshal(map[string]any{
				"pattern": "func",
				"glob":    "*.go",
			})

			result, err := tools.Execute(ctx, "grep", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("func"))
		})

		It("finds pattern in specific path", func() {
			args, _ := json.Marshal(map[string]any{
				"pattern": "Helper",
				"path":    "src/util",
			})

			result, err := tools.Execute(ctx, "grep", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("helper.go"))
		})

		It("returns no matches message", func() {
			args, _ := json.Marshal(map[string]any{
				"pattern": "nonexistent_xyz_123",
			})

			result, err := tools.Execute(ctx, "grep", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("No matches"))
		})

		It("returns error for missing pattern", func() {
			args, _ := json.Marshal(map[string]any{})

			result, err := tools.Execute(ctx, "grep", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("pattern is required"))
		})
	})

	Describe("Glob Tool", func() {
		It("finds files by pattern", func() {
			args, _ := json.Marshal(map[string]any{
				"pattern": "*.go",
			})

			result, err := tools.Execute(ctx, "glob", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("main.go"))
			Expect(result).To(ContainSubstring("helper.go"))
		})

		It("finds files in specific path", func() {
			args, _ := json.Marshal(map[string]any{
				"pattern": "*.go",
				"path":    "src/util",
			})

			result, err := tools.Execute(ctx, "glob", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("helper.go"))
			Expect(result).NotTo(ContainSubstring("main.go"))
		})

		It("returns no matches for non-matching pattern", func() {
			args, _ := json.Marshal(map[string]any{
				"pattern": "*.xyz",
			})

			result, err := tools.Execute(ctx, "glob", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("No files match"))
		})

		It("returns error for missing pattern", func() {
			args, _ := json.Marshal(map[string]any{})

			result, err := tools.Execute(ctx, "glob", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("pattern is required"))
		})

		It("returns error for path outside repo", func() {
			args, _ := json.Marshal(map[string]any{
				"pattern": "*.go",
				"path":    "../../../",
			})

			result, err := tools.Execute(ctx, "glob", string(args))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("outside repository"))
		})
	})

	Describe("Bash Tool", func() {
		Describe("Allowed Commands", func() {
			It("executes ls command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "ls src",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("main.go"))
				Expect(result).To(ContainSubstring("util"))
			})

			It("executes find command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "find . -name '*.go'",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("main.go"))
			})

			It("executes git log command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git log --oneline -1 2>/dev/null || echo 'not a git repo'",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeEmpty())
			})

			It("executes tree command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "tree -L 1 2>/dev/null || ls",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(ContainSubstring("Command blocked"))
			})
		})

		Describe("Blocked Commands", func() {
			It("blocks rm command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "rm src/main.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks git push command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git push origin main",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks git commit command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git commit -m 'test'",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks output redirection", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "ls > output.txt",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
				Expect(result).To(ContainSubstring("redirection"))
			})

			It("blocks unlisted commands", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "curl https://example.com",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
				Expect(result).To(ContainSubstring("not in allowed list"))
			})
		})

		Describe("Edge Cases", func() {
			It("handles empty command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("command is required"))
			})

			It("handles whitespace-only command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "   ",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("command is required"))
			})
		})

		Describe("Output Limits", func() {
			It("truncates very large output", func() {
				// Create many files to generate large ls output
				for i := 0; i < 200; i++ {
					Expect(os.WriteFile(filepath.Join(tempDir, "file"+string(rune('a'+i%26))+".txt"), []byte("content"), 0o644)).To(Succeed())
				}

				args, _ := json.Marshal(map[string]any{
					"command": "find . -type f",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				// Should not error, output may be truncated for very large results
				Expect(result).NotTo(BeEmpty())
			})
		})
	})

	Describe("Unknown Tool", func() {
		It("returns error for unknown tool", func() {
			args, _ := json.Marshal(map[string]any{
				"foo": "bar",
			})

			_, err := tools.Execute(ctx, "nonexistent_tool", string(args))

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown tool"))
		})
	})
})
