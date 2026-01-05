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
		//   .git/
		//     config
		//   README.md
		Expect(os.MkdirAll(filepath.Join(tempDir, "src", "util"), 0o755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, "src", "main.go"), []byte("package main"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, "src", "util", "helper.go"), []byte("package util"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, ".git", "config"), []byte("[core]"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Test"), 0o644)).To(Succeed())

		// Create tools with nil arango client (not needed for bash tests)
		tools = brain.NewExploreTools(tempDir, nil)
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("Bash Tool", func() {
		Describe("Allowed Commands", func() {
			It("executes cat command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "cat src/main.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("package main"))
			})

			It("executes head command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "head -1 src/main.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("package main"))
			})

			It("executes tail command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "tail -1 src/main.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("package main"))
			})

			It("executes ls command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "ls src",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("main.go"))
				Expect(result).To(ContainSubstring("util"))
			})

			It("executes ls without arguments", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "ls",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("src"))
				Expect(result).To(ContainSubstring("README.md"))
			})

			It("executes tree command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "tree -L 1",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				// tree command should be allowed
				Expect(result).NotTo(ContainSubstring("Command blocked"))
			})

			It("executes wc command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "wc -c src/main.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				// "package main" is 12 bytes
				Expect(result).To(ContainSubstring("12"))
			})

			It("executes sed command for line range", func() {
				// Create a multi-line file
				Expect(os.WriteFile(filepath.Join(tempDir, "multi.txt"), []byte("line1\nline2\nline3\nline4\nline5\n"), 0o644)).To(Succeed())

				args, _ := json.Marshal(map[string]any{
					"command": "sed -n '2,4p' multi.txt",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("line2"))
				Expect(result).To(ContainSubstring("line3"))
				Expect(result).To(ContainSubstring("line4"))
				Expect(result).NotTo(ContainSubstring("line1"))
				Expect(result).NotTo(ContainSubstring("line5"))
			})

			It("executes find command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "find . -name '*.go'",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("main.go"))
				Expect(result).To(ContainSubstring("helper.go"))
			})

			It("executes file command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "file src/main.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Or(
					ContainSubstring("ASCII text"),
					ContainSubstring("text"),
				))
			})

			It("executes git log command", func() {
				// This may fail if not a git repo, but should be allowed
				args, _ := json.Marshal(map[string]any{
					"command": "git log --oneline -1 2>/dev/null || echo 'not a git repo'",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				// Either shows commit or "not a git repo" - both are valid
				Expect(result).NotTo(BeEmpty())
			})

			It("executes git status command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git status 2>/dev/null || echo 'not a git repo'",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeEmpty())
			})

			It("executes git diff command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git diff 2>/dev/null || echo 'not a git repo'",
				})

				_, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				// Empty diff or error message - both are valid
			})

			It("executes git blame command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git blame src/main.go 2>/dev/null || echo 'not a git repo'",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeEmpty())
			})

			It("executes stat command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "stat src/main.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("main.go"))
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
				Expect(result).To(ContainSubstring("write operation"))
			})

			It("blocks mv command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "mv src/main.go src/new.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks cp command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "cp src/main.go src/copy.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks mkdir command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "mkdir newdir",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks touch command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "touch newfile.txt",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks chmod command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "chmod 755 src/main.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks echo command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "echo 'malicious' > file.txt",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks printf command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "printf 'data' > file.txt",
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

			It("blocks git checkout command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git checkout main",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks git reset command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git reset --hard HEAD",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks git rebase command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git rebase main",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks git merge command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git merge feature",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks git pull command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git pull origin main",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks git stash command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git stash",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks git clean command", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "git clean -fd",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
			})

			It("blocks output redirection with >", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "cat src/main.go > copy.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
				Expect(result).To(ContainSubstring("redirection"))
			})

			It("blocks output redirection with >>", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "cat src/main.go >> copy.go",
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

		Describe("Path Security", func() {
			It("blocks absolute paths outside repo root", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "cat /etc/passwd",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
				Expect(result).To(ContainSubstring("path outside repository"))
			})

			It("blocks access to parent directories via absolute path", func() {
				parentPath := filepath.Dir(tempDir)
				args, _ := json.Marshal(map[string]any{
					"command": "cat " + parentPath + "/somefile",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("Command blocked"))
				Expect(result).To(ContainSubstring("path outside repository"))
			})

			It("allows relative paths within repo", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "cat src/main.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("package main"))
			})

			It("allows nested relative paths within repo", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "cat src/util/helper.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("package util"))
			})
		})

		Describe("Grep Handling", func() {
			It("returns 'No matches found' for grep with no results", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "grep 'nonexistent_string_xyz' src/main.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("No matches found"))
			})

			It("returns matches for successful grep", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "grep 'package' src/main.go",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("package main"))
			})

			It("handles grep with multiple matches", func() {
				// Create a file with multiple matching lines
				Expect(os.WriteFile(filepath.Join(tempDir, "multi.txt"), []byte("line1\nline2\nline3\n"), 0o644)).To(Succeed())

				args, _ := json.Marshal(map[string]any{
					"command": "grep 'line' multi.txt",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("line1"))
				Expect(result).To(ContainSubstring("line2"))
				Expect(result).To(ContainSubstring("line3"))
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

			It("handles command with leading/trailing whitespace", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "  cat src/main.go  ",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("package main"))
			})

			It("handles piped commands", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "cat src/main.go | head -1",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("package main"))
			})

			It("handles command that produces error output", func() {
				args, _ := json.Marshal(map[string]any{
					"command": "cat nonexistent_file.txt",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("No such file"))
			})

			It("handles paths with spaces", func() {
				spacePath := filepath.Join(tempDir, "path with spaces")
				Expect(os.MkdirAll(spacePath, 0o755)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(spacePath, "file.txt"), []byte("test content"), 0o644)).To(Succeed())

				args, _ := json.Marshal(map[string]any{
					"command": `cat "path with spaces/file.txt"`,
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("test content"))
			})
		})

		Describe("Output Limits", func() {
			It("truncates very large output", func() {
				// Create a large file
				largeContent := make([]byte, 20000) // 20KB - exceeds 10KB limit
				for i := range largeContent {
					largeContent[i] = byte('a' + (i % 26))
				}
				Expect(os.WriteFile(filepath.Join(tempDir, "large.txt"), largeContent, 0o644)).To(Succeed())

				args, _ := json.Marshal(map[string]any{
					"command": "cat large.txt",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(len(result)).To(BeNumerically("<", 15000)) // Should be truncated
				Expect(result).To(ContainSubstring("truncated"))
			})

			It("preserves output under the limit", func() {
				smallContent := "small content"
				Expect(os.WriteFile(filepath.Join(tempDir, "small.txt"), []byte(smallContent), 0o644)).To(Succeed())

				args, _ := json.Marshal(map[string]any{
					"command": "cat small.txt",
				})

				result, err := tools.Execute(ctx, "bash", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("small content"))
				Expect(result).NotTo(ContainSubstring("truncated"))
			})
		})
	})

	Describe("Codegraph Tool", func() {
		Describe("Parameter Validation", func() {
			It("returns error for missing symbol in find operation", func() {
				args, _ := json.Marshal(map[string]any{
					"operation": "find",
				})

				result, err := tools.Execute(ctx, "codegraph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("symbol"))
				Expect(result).To(ContainSubstring("required"))
			})

			It("returns error for missing symbol in callers operation", func() {
				args, _ := json.Marshal(map[string]any{
					"operation": "callers",
				})

				result, err := tools.Execute(ctx, "codegraph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("symbol"))
				Expect(result).To(ContainSubstring("required"))
			})

			It("returns error for missing file in symbols operation", func() {
				args, _ := json.Marshal(map[string]any{
					"operation": "symbols",
				})

				result, err := tools.Execute(ctx, "codegraph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("file"))
				Expect(result).To(ContainSubstring("required"))
			})

			It("returns error for invalid operation", func() {
				args, _ := json.Marshal(map[string]any{
					"operation": "invalid_op",
					"symbol":    "Test",
				})

				result, err := tools.Execute(ctx, "codegraph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("invalid_operation"))
				Expect(result).To(ContainSubstring("invalid_op"))
			})
		})

		Describe("Unsupported Language Detection", func() {
			It("returns unsupported language error for TypeScript files", func() {
				args, _ := json.Marshal(map[string]any{
					"operation": "symbols",
					"file":      "src/components/Button.tsx",
				})

				result, err := tools.Execute(ctx, "codegraph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("unsupported_language"))
				Expect(result).To(ContainSubstring("TypeScript"))
				Expect(result).To(ContainSubstring("Go, Python"))
			})

			It("returns unsupported language error for JavaScript files", func() {
				args, _ := json.Marshal(map[string]any{
					"operation": "symbols",
					"file":      "src/index.js",
				})

				result, err := tools.Execute(ctx, "codegraph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("unsupported_language"))
				Expect(result).To(ContainSubstring("JavaScript"))
			})

			It("returns unsupported language error for Rust files", func() {
				args, _ := json.Marshal(map[string]any{
					"operation": "symbols",
					"file":      "src/main.rs",
				})

				result, err := tools.Execute(ctx, "codegraph", string(args))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("unsupported_language"))
				Expect(result).To(ContainSubstring("Rust"))
			})
		})
	})
})
