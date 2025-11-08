package process

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/humanbeeng/lepo/prototypes/codegraph/extract"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

// Neo4jIngestor persists extracted nodes and relationships directly into Neo4j.
type Neo4jIngestor struct {
	cfg Neo4jConfig
}

// NewNeo4jIngestor validates the provided configuration and creates an ingestor.
func NewNeo4jIngestor(cfg Neo4jConfig) (*Neo4jIngestor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Neo4jIngestor{cfg: cfg}, nil
}

// Ingest connects to Neo4j and writes the extracted result into the graph.
func (i *Neo4jIngestor) Ingest(ctx context.Context, res extract.ExtractNodesResult) error {
	driver, err := neo4j.NewDriver(i.cfg.URI, neo4j.BasicAuth(i.cfg.Username, i.cfg.Password, ""))
	if err != nil {
		return fmt.Errorf("create neo4j driver: %w", err)
	}
	defer func() {
		if closeErr := driver.Close(ctx); closeErr != nil {
			slog.Error("failed closing neo4j driver", "err", closeErr)
		}
	}()

	slog.Info("Connecting to Neo4j", "uri", i.cfg.URI)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("verify neo4j connectivity: %w", err)
	}
	slog.Info("Neo4j connectivity verified", "uri", i.cfg.URI)

	exec := func(stage string, rowCount int, query string, params map[string]any) error {
		slog.Info("Executing Cypher query", "stage", stage, "rows", rowCount, "database", i.cfg.Database)
		options := []neo4j.ExecuteQueryConfigurationOption{}
		if i.cfg.Database != "" {
			options = append(options, neo4j.ExecuteQueryWithDatabase(i.cfg.Database))
		}
		_, execErr := neo4j.ExecuteQuery(ctx, driver, query, params, neo4j.EagerResultTransformer, options...)
		if execErr != nil {
			slog.Error("Cypher execution failed", "stage", stage, "err", execErr)
			return execErr
		}
		slog.Info("Cypher execution completed", "stage", stage, "rows", rowCount)
		return nil
	}

	stages := []struct {
		name string
		run  func() error
	}{
		{"indexes:nodes", func() error { return i.ensureNodeIndexes(exec) }},
		{"nodes:namespaces", func() error { return i.ingestNamespaces(exec, res.Namespaces) }},
		{"nodes:files", func() error { return i.ingestFiles(exec, res.Files) }},
		{"nodes:imports", func() error { return i.ingestImports(exec, res.Files) }},
		{"nodes:type_decls", func() error { return i.ingestTypeDecls(exec, res.TypeDecls) }},
		{"nodes:interfaces", func() error { return i.ingestInterfaces(exec, res.Interfaces) }},
		{"nodes:named", func() error { return i.ingestNamed(exec, res.NamedTypes) }},
		{"nodes:members", func() error { return i.ingestMembers(exec, res.Members) }},
		{"nodes:functions", func() error { return i.ingestFunctions(exec, res.Functions) }},
		{"relationships:calls", func() error { return i.ingestCalls(exec, res.Functions) }},
		{"relationships:returns", func() error { return i.ingestReturns(exec, res.Functions) }},
		{"relationships:params", func() error { return i.ingestParams(exec, res.Functions) }},
		{"relationships:implements", func() error { return i.ingestImplements(exec, res.TypeDecls) }},
		{"relationships:file_imports", func() error { return i.ingestFileImports(exec, res.Files) }},
	}

	progress := newProgressBar(len(stages))
	for _, stage := range stages {
		if err := progress.runStage(stage.name, stage.run); err != nil {
			return err
		}
	}

	slog.Info("Neo4j ingestion completed")
	return nil
}

type queryExecutor func(stage string, rowCount int, query string, params map[string]any) error

// executeBatched splits rows into batches and executes the query for each batch
func executeBatched(exec queryExecutor, stage string, query string, rows []map[string]any, batchSize int) error {
	if len(rows) == 0 {
		return nil
	}

	totalBatches := (len(rows) + batchSize - 1) / batchSize
	slog.Info("Executing in batches", "stage", stage, "total_rows", len(rows), "batch_size", batchSize, "batches", totalBatches)

	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]
		batchNum := (i / batchSize) + 1
		slog.Debug("Executing batch", "stage", stage, "batch", batchNum, "of", totalBatches, "rows", len(batch))

		if err := exec(stage, len(batch), query, map[string]any{"rows": batch}); err != nil {
			return fmt.Errorf("batch %d/%d failed: %w", batchNum, totalBatches, err)
		}
	}

	slog.Info("All batches completed", "stage", stage, "total_rows", len(rows))
	return nil
}

func (i *Neo4jIngestor) ingestNamespaces(exec queryExecutor, namespaces []extract.Namespace) error {
	stage := "nodes:namespaces"
	if len(namespaces) == 0 {
		slog.Info("No data to ingest", "stage", stage)
		return nil
	}
	slog.Info("Preparing namespace rows", "stage", stage, "count", len(namespaces))
	rows := make([]map[string]any, 0, len(namespaces))
	for _, ns := range namespaces {
		if ns.Name == "" {
			continue
		}
		rows = append(rows, map[string]any{
			"name": ns.Name,
			"kind": "namespace",
		})
	}
	if len(rows) == 0 {
		slog.Info("No namespace rows prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared namespace rows", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MERGE (:Node {name: row.name, kind: row.kind})
`
	return exec(stage, len(rows), query, map[string]any{"rows": rows})
}

func (i *Neo4jIngestor) ingestFiles(exec queryExecutor, files map[string]extract.File) error {
	stage := "nodes:files"
	if len(files) == 0 {
		slog.Info("No file nodes to ingest", "stage", stage)
		return nil
	}
	slog.Info("Preparing file nodes", "stage", stage, "count", len(files))
	rows := make([]map[string]any, 0, len(files))
	for _, file := range files {
		if file.Filename == "" {
			continue
		}
		rows = append(rows, map[string]any{
			"file":      file.Filename,
			"namespace": file.Namespace.Name,
			"language":  file.Language,
			"kind":      "file",
		})
	}
	if len(rows) == 0 {
		slog.Info("No file rows prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared file rows", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MERGE (n:Node {file: row.file})
SET n.namespace = row.namespace,
    n.language = row.language,
    n.kind = row.kind
`
	return exec(stage, len(rows), query, map[string]any{"rows": rows})
}

func (i *Neo4jIngestor) ingestImports(exec queryExecutor, files map[string]extract.File) error {
	stage := "nodes:imports"
	slog.Info("Preparing import nodes", "stage", stage, "files", len(files))
	rows := make([]map[string]any, 0)
	for _, file := range files {
		for _, imp := range file.Imports {
			rows = append(rows, map[string]any{
				"name":      coalesce(imp.Name),
				"namespace": imp.Path,
				"kind":      "import",
				"comment":   coalesceDoc(imp.Doc.Comment),
			})
		}
	}
	if len(rows) == 0 {
		slog.Info("No import rows prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared import rows", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MERGE (n:Node {name: row.name, namespace: row.namespace, kind: row.kind})
SET n.comment = row.comment
`
	return exec(stage, len(rows), query, map[string]any{"rows": rows})
}

func (i *Neo4jIngestor) ingestTypeDecls(exec queryExecutor, decls map[string]extract.TypeDecl) error {
	stage := "nodes:type_decls"
	if len(decls) == 0 {
		slog.Info("No type declarations to ingest", "stage", stage)
		return nil
	}
	slog.Info("Preparing type declaration rows", "stage", stage, "count", len(decls))
	rows := make([]map[string]any, 0, len(decls))
	for qname, decl := range decls {
		if qname == "" {
			continue
		}
		rows = append(rows, map[string]any{
			"qualified_name": qname,
			"name":           decl.Name,
			"type":           decl.TypeQName,
			"underlying":     decl.Underlying,
			"kind":           string(decl.Kind),
			"code":           decl.Code,
			"doc":            coalesceDoc(decl.Doc.Comment),
			"namespace":      decl.Namespace.Name,
		})
	}
	if len(rows) == 0 {
		slog.Info("No type declaration rows prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared type declaration rows", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MERGE (n:Node {qualified_name: row.qualified_name})
SET n.name = row.name,
    n.type = row.type,
    n.underlying_type = row.underlying,
    n.kind = row.kind,
    n.code = row.code,
    n.doc = row.doc,
    n.namespace = row.namespace
`
	return exec(stage, len(rows), query, map[string]any{"rows": rows})
}

func (i *Neo4jIngestor) ingestInterfaces(exec queryExecutor, interfaces map[string]extract.TypeDecl) error {
	stage := "nodes:interfaces"
	if len(interfaces) == 0 {
		slog.Info("No interface declarations to ingest", "stage", stage)
		return nil
	}
	slog.Info("Preparing interface rows", "stage", stage, "count", len(interfaces))
	rows := make([]map[string]any, 0, len(interfaces))
	for qname, iface := range interfaces {
		if qname == "" {
			continue
		}
		rows = append(rows, map[string]any{
			"qualified_name": qname,
			"name":           iface.Name,
			"type":           iface.TypeQName,
			"underlying":     iface.Underlying,
			"code":           iface.Code,
			"doc":            coalesceDoc(iface.Doc.Comment),
			"namespace":      iface.Namespace.Name,
			"kind":           string(iface.Kind),
		})
	}
	if len(rows) == 0 {
		slog.Info("No interface rows prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared interface rows", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MERGE (n:Node {qualified_name: row.qualified_name})
SET n.name = row.name,
    n.type = row.type,
    n.underlying_type = row.underlying,
    n.kind = row.kind,
    n.code = row.code,
    n.doc = row.doc,
    n.namespace = row.namespace
`
	return exec(stage, len(rows), query, map[string]any{"rows": rows})
}

func (i *Neo4jIngestor) ingestNamed(exec queryExecutor, named map[string]extract.Named) error {
	stage := "nodes:named"
	if len(named) == 0 {
		slog.Info("No named types to ingest", "stage", stage)
		return nil
	}
	slog.Info("Preparing named type rows", "stage", stage, "count", len(named))
	rows := make([]map[string]any, 0, len(named))
	for qname, n := range named {
		if qname == "" {
			continue
		}
		rows = append(rows, map[string]any{
			"qualified_name": qname,
			"name":           n.Name,
			"type":           n.TypeQName,
			"underlying":     n.Underlying,
			"kind":           "named",
			"code":           n.Code,
			"doc":            coalesceDoc(n.Doc.Comment),
			"namespace":      n.Namespace.Name,
		})
	}
	if len(rows) == 0 {
		slog.Info("No named type rows prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared named type rows", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MERGE (n:Node {qualified_name: row.qualified_name})
SET n.name = row.name,
    n.type = row.type,
    n.underlying_type = row.underlying,
    n.kind = row.kind,
    n.code = row.code,
    n.doc = row.doc,
    n.namespace = row.namespace
`
	return exec(stage, len(rows), query, map[string]any{"rows": rows})
}

func (i *Neo4jIngestor) ingestMembers(exec queryExecutor, members map[string]extract.Member) error {
	stage := "nodes:members"
	if len(members) == 0 {
		slog.Info("No members to ingest", "stage", stage)
		return nil
	}
	slog.Info("Preparing member rows", "stage", stage, "count", len(members))
	rows := make([]map[string]any, 0, len(members))
	for qname, member := range members {
		if qname == "" {
			continue
		}
		rows = append(rows, map[string]any{
			"qualified_name":        qname,
			"name":                  member.Name,
			"type":                  member.TypeQName,
			"parent_qualified_name": member.ParentQName,
			"kind":                  "member",
			"code":                  member.Code,
			"doc":                   coalesceDoc(member.Doc.Comment),
			"namespace":             member.Namespace.Name,
		})
	}
	if len(rows) == 0 {
		slog.Info("No member rows prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared member rows", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MERGE (n:Node {qualified_name: row.qualified_name})
SET n.name = row.name,
    n.type = row.type,
    n.parent_qualified_name = row.parent_qualified_name,
    n.kind = row.kind,
    n.code = row.code,
    n.doc = row.doc,
    n.namespace = row.namespace
`
	return exec(stage, len(rows), query, map[string]any{"rows": rows})
}

func (i *Neo4jIngestor) ingestFunctions(exec queryExecutor, functions map[string]extract.Function) error {
	stage := "nodes:functions"
	if len(functions) == 0 {
		slog.Info("No functions to ingest", "stage", stage)
		return nil
	}
	slog.Info("Preparing function rows", "stage", stage, "count", len(functions))
	rows := make([]map[string]any, 0, len(functions))
	query := `
UNWIND $rows AS row
MERGE (n:Node {qualified_name: row.qualified_name})
SET n += row
`
	for qname, fn := range functions {
		if qname == "" || fn.Filepath == "" {
			continue
		}
		rows = append(rows, map[string]any{
			"qualified_name":        qname,
			"name":                  fn.Name,
			"kind":                  "function",
			"code":                  fn.Code,
			"doc":                   coalesceDoc(fn.Doc.Comment),
			"file":                  fn.Filepath,
			"namespace":             fn.Namespace.Name,
			"parent_qualified_name": fn.ParentQName,
		})
	}
	if len(rows) == 0 {
		slog.Info("No function rows prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared function rows", "stage", stage, "rows", len(rows))
	return exec(stage, len(rows), query, map[string]any{"rows": rows})
}

func (i *Neo4jIngestor) ensureNodeIndexes(exec queryExecutor) error {
	stage := "indexes:nodes"
	slog.Info("Ensuring node indexes", "stage", stage)
	indexes := []struct {
		name  string
		query string
	}{
		{
			name:  "node_qualified_name",
			query: `CREATE INDEX node_qualified_name IF NOT EXISTS FOR (n:Node) ON (n.qualified_name)`,
		},
		{
			name:  "node_kind",
			query: `CREATE INDEX node_kind IF NOT EXISTS FOR (n:Node) ON (n.kind)`,
		},
		{
			name:  "node_file",
			query: `CREATE INDEX node_file IF NOT EXISTS FOR (n:Node) ON (n.file)`,
		},
		{
			name:  "node_name",
			query: `CREATE INDEX node_name IF NOT EXISTS FOR (n:Node) ON (n.name)`,
		},
		{
			name:  "node_namespace",
			query: `CREATE INDEX node_namespace IF NOT EXISTS FOR (n:Node) ON (n.namespace)`,
		},
		{
			name:  "node_qualified_name_kind",
			query: `CREATE INDEX node_qualified_name_kind IF NOT EXISTS FOR (n:Node) ON (n.qualified_name, n.kind)`,
		},
		{
			name:  "node_file_kind",
			query: `CREATE INDEX node_file_kind IF NOT EXISTS FOR (n:Node) ON (n.file, n.kind)`,
		},
		{
			name:  "node_namespace_kind",
			query: `CREATE INDEX node_namespace_kind IF NOT EXISTS FOR (n:Node) ON (n.namespace, n.kind)`,
		},
	}
	for _, idx := range indexes {
		slog.Info("Ensuring index", "stage", stage, "index", idx.name)
		if err := exec(stage, 0, idx.query, nil); err != nil {
			return err
		}
	}
	slog.Info("Node indexes ensured", "stage", stage)
	return nil
}

func (i *Neo4jIngestor) ingestCalls(exec queryExecutor, functions map[string]extract.Function) error {
	stage := "relationships:calls"
	slog.Info("Preparing call relationships", "stage", stage, "functions", len(functions))
	rows := make([]map[string]any, 0)
	for _, fn := range functions {
		if fn.QName == "" {
			continue
		}
		for _, callee := range fn.Calls {
			if callee == "" {
				continue
			}
			rows = append(rows, map[string]any{"from": fn.QName, "to": callee})
		}
	}
	if len(rows) == 0 {
		slog.Info("No call relationships prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared call relationships", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MATCH (src:Node {qualified_name: row.from, kind: "function"})
MATCH (dst:Node {qualified_name: row.to, kind: "function"})
MERGE (src)-[:CALLS]->(dst)
`
	return executeBatched(exec, stage, query, rows, 10000)
}

func (i *Neo4jIngestor) ingestReturns(exec queryExecutor, functions map[string]extract.Function) error {
	stage := "relationships:returns"
	slog.Info("Preparing return relationships", "stage", stage, "functions", len(functions))
	rows := make([]map[string]any, 0)
	for _, fn := range functions {
		if fn.QName == "" {
			continue
		}
		for _, ret := range fn.ReturnQNames {
			if ret == "" {
				continue
			}
			rows = append(rows, map[string]any{"from": fn.QName, "to": ret})
		}
	}
	if len(rows) == 0 {
		slog.Info("No return relationships prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared return relationships", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MATCH (src:Node {qualified_name: row.from})
MATCH (dst:Node {qualified_name: row.to})
MERGE (src)-[:RETURNS]->(dst)
`
	return executeBatched(exec, stage, query, rows, 10000)
}

func (i *Neo4jIngestor) ingestParams(exec queryExecutor, functions map[string]extract.Function) error {
	stage := "relationships:params"
	slog.Info("Preparing parameter relationships", "stage", stage, "functions", len(functions))
	rows := make([]map[string]any, 0)
	for _, fn := range functions {
		if fn.QName == "" {
			continue
		}
		for _, param := range fn.ParamQNames {
			if param == "" {
				continue
			}
			rows = append(rows, map[string]any{"from": param, "to": fn.QName})
		}
	}
	if len(rows) == 0 {
		slog.Info("No parameter relationships prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared parameter relationships", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MATCH (src:Node {qualified_name: row.from})
MATCH (dst:Node {qualified_name: row.to})
MERGE (src)-[:PARAM_OF]->(dst)
`
	return executeBatched(exec, stage, query, rows, 10000)
}

func (i *Neo4jIngestor) ingestImplements(exec queryExecutor, decls map[string]extract.TypeDecl) error {
	stage := "relationships:implements"
	slog.Info("Preparing implements relationships", "stage", stage, "type_decls", len(decls))
	rows := make([]map[string]any, 0)
	for qname, decl := range decls {
		if qname == "" {
			continue
		}
		for _, iface := range decl.ImplementsQName {
			if iface == "" {
				continue
			}
			rows = append(rows, map[string]any{"from": qname, "to": iface})
		}
	}
	if len(rows) == 0 {
		slog.Info("No implements relationships prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared implements relationships", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MATCH (src:Node {qualified_name: row.from})
WHERE src.kind IN ["struct", "alias"]
MATCH (dst:Node {qualified_name: row.to, kind: "interface"})
MERGE (src)-[:IMPLEMENTS]->(dst)
`
	return executeBatched(exec, stage, query, rows, 10000)
}

func (i *Neo4jIngestor) ingestFileImports(exec queryExecutor, files map[string]extract.File) error {
	stage := "relationships:file_imports"
	slog.Info("Preparing file import relationships", "stage", stage, "files", len(files))
	rows := make([]map[string]any, 0)
	for _, file := range files {
		if file.Filename == "" {
			continue
		}
		for _, imp := range file.Imports {
			if imp.Path == "" {
				continue
			}
			rows = append(rows, map[string]any{
				"from": file.Filename,
				"to":   imp.Path,
			})
		}
	}
	if len(rows) == 0 {
		slog.Info("No file import relationships prepared", "stage", stage)
		return nil
	}
	slog.Info("Prepared file import relationships", "stage", stage, "rows", len(rows))
	query := `
UNWIND $rows AS row
MATCH (src:Node {file: row.from, kind: "file"})
MATCH (dst:Node {namespace: row.to, kind: "import"})
MERGE (src)-[:IMPORTS]->(dst)
`
	return executeBatched(exec, stage, query, rows, 10000)
}

type progressBar struct {
	total   int
	current int
	width   int
	mu      sync.Mutex
}

func newProgressBar(total int) *progressBar {
	if total <= 0 {
		total = 1
	}
	return &progressBar{total: total, width: 40}
}

func (p *progressBar) runStage(stage string, fn func() error) error {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.animate(stage, done)
	}()
	err := fn()
	close(done)
	wg.Wait()
	if err != nil {
		p.failStage(stage)
		return err
	}
	p.completeStage(stage)
	return nil
}

func (p *progressBar) animate(stage string, done <-chan struct{}) {
	frames := []rune{'|', '/', '-', '\\'}
	index := 0
	for {
		select {
		case <-done:
			return
		default:
			p.render(stage, frames[index])
			index = (index + 1) % len(frames)
			time.Sleep(120 * time.Millisecond)
		}
	}
}

func (p *progressBar) completeStage(stage string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current++
	if p.current > p.total {
		p.current = p.total
	}
	p.printLine(p.formatLine(stage+" done", p.current), p.current == p.total)
}

func (p *progressBar) failStage(stage string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	line := p.formatLine(stage+" failed", p.current)
	p.printLine(line, true)
}

func (p *progressBar) render(stage string, spinner rune) {
	p.mu.Lock()
	defer p.mu.Unlock()
	progress := p.current
	if progress < p.total {
		progress++
	}
	line := p.formatLine(fmt.Sprintf("%s %c", stage, spinner), progress)
	p.printLine(line, false)
}

func (p *progressBar) formatLine(status string, progress int) string {
	if progress < 0 {
		progress = 0
	}
	if progress > p.total {
		progress = p.total
	}
	bar := p.bar(progress)
	return fmt.Sprintf("[%s] %d/%d %s", bar, progress, p.total, status)
}

func (p *progressBar) bar(progress int) string {
	if progress < 0 {
		progress = 0
	}
	if p.total <= 0 {
		return strings.Repeat(" ", p.width)
	}
	if progress > p.total {
		progress = p.total
	}
	filled := int(float64(progress) / float64(p.total) * float64(p.width))
	if filled > p.width {
		filled = p.width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("=", filled) + strings.Repeat(" ", p.width-filled)
}

func (p *progressBar) printLine(content string, newline bool) {
	totalWidth := p.width + 50
	fmt.Fprintf(os.Stdout, "\r%-*s", totalWidth, content)
	if newline {
		fmt.Fprint(os.Stdout, "\n")
	}
}

func coalesce(value string) string {
	return strings.TrimSpace(value)
}

func coalesceDoc(comment string) string {
	return coalesce(comment)
}
