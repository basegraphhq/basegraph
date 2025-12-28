package process

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"basegraph.app/relay/common/arangodb"
	"basegraph.app/relay/common/typesense"
	"github.com/humanbeeng/lepo/prototypes/codegraph/extract"
)

// Ingestor handles ingestion of extracted code into Typesense and ArangoDB.
type Ingestor struct {
	ts     typesense.Client
	arango arangodb.Client
}

// NewIngestor creates a new Ingestor with the provided clients.
func NewIngestor(ts typesense.Client, arango arangodb.Client) *Ingestor {
	return &Ingestor{
		ts:     ts,
		arango: arango,
	}
}

// Ingest processes the extraction result and ingests it into both databases.
func (i *Ingestor) Ingest(ctx context.Context, res extract.ExtractNodesResult) error {
	start := time.Now()

	// Step 1: Setup databases
	slog.Info("Setting up ArangoDB schema")
	if err := i.arango.EnsureDatabase(ctx); err != nil {
		return fmt.Errorf("ensure database: %w", err)
	}
	if err := i.arango.EnsureCollections(ctx); err != nil {
		return fmt.Errorf("ensure collections: %w", err)
	}
	if err := i.arango.EnsureGraph(ctx); err != nil {
		return fmt.Errorf("ensure graph: %w", err)
	}

	slog.Info("Setting up Typesense collection")
	if err := i.ts.CreateCollection(ctx); err != nil {
		// Ignore error if collection already exists
		slog.Debug("Create collection result (may already exist)", "err", err)
	}

	// Step 2: Ingest nodes into Typesense (for full-text search)
	slog.Info("Ingesting nodes into Typesense")
	if err := i.ingestToTypesense(ctx, res); err != nil {
		return fmt.Errorf("typesense ingestion: %w", err)
	}

	// Step 3: Ingest edges into ArangoDB (for graph traversal)
	slog.Info("Ingesting nodes and edges into ArangoDB")
	if err := i.ingestToArangoDB(ctx, res); err != nil {
		return fmt.Errorf("arangodb ingestion: %w", err)
	}

	slog.Info("Ingestion completed",
		"duration_ms", time.Since(start).Milliseconds())
	return nil
}

func (i *Ingestor) ingestToTypesense(ctx context.Context, res extract.ExtractNodesResult) error {
	var docs []typesense.Document

	// Functions
	for qname, fn := range res.Functions {
		if qname == "" || fn.Filepath == "" {
			continue
		}
		docs = append(docs, typesense.Document{
			QName:        qname,
			Name:         fn.Name,
			Kind:         kindForFunction(fn),
			Code:         fn.Code,
			Doc:          fn.Doc.Comment,
			Filepath:     fn.Filepath,
			Namespace:    fn.Namespace.Name,
			Language:     extract.Go,
			Pos:          fn.Pos,
			End:          fn.End,
			NameVariants: generateNameVariants(fn.Name),
		})
	}

	// Type declarations (structs)
	for qname, decl := range res.TypeDecls {
		if qname == "" {
			continue
		}
		docs = append(docs, typesense.Document{
			QName:        qname,
			Name:         decl.Name,
			Kind:         string(decl.Kind),
			Code:         decl.Code,
			Doc:          decl.Doc.Comment,
			Filepath:     decl.Filepath,
			Namespace:    decl.Namespace.Name,
			Language:     extract.Go,
			Pos:          decl.Pos,
			End:          decl.End,
			NameVariants: generateNameVariants(decl.Name),
		})
	}

	// Interfaces
	for qname, iface := range res.Interfaces {
		if qname == "" {
			continue
		}
		docs = append(docs, typesense.Document{
			QName:        qname,
			Name:         iface.Name,
			Kind:         "interface",
			Code:         iface.Code,
			Doc:          iface.Doc.Comment,
			Filepath:     iface.Filepath,
			Namespace:    iface.Namespace.Name,
			Language:     extract.Go,
			Pos:          iface.Pos,
			End:          iface.End,
			NameVariants: generateNameVariants(iface.Name),
		})
	}

	// Named types (type aliases)
	for qname, named := range res.NamedTypes {
		if qname == "" {
			continue
		}
		docs = append(docs, typesense.Document{
			QName:        qname,
			Name:         named.Name,
			Kind:         "alias",
			Code:         named.Code,
			Doc:          named.Doc.Comment,
			Filepath:     named.Filepath,
			Namespace:    named.Namespace.Name,
			Language:     extract.Go,
			Pos:          named.Pos,
			End:          named.End,
			NameVariants: generateNameVariants(named.Name),
		})
	}

	// Members (struct fields)
	for qname, member := range res.Members {
		if qname == "" {
			continue
		}
		docs = append(docs, typesense.Document{
			QName:        qname,
			Name:         member.Name,
			Kind:         "member",
			Code:         member.Code,
			Doc:          member.Doc.Comment,
			Filepath:     member.Filepath,
			Namespace:    member.Namespace.Name,
			Language:     extract.Go,
			Pos:          member.Pos,
			End:          member.End,
			NameVariants: generateNameVariants(member.Name),
		})
	}

	// Variables
	for qname, v := range res.Vars {
		if qname == "" {
			continue
		}
		docs = append(docs, typesense.Document{
			QName:        qname,
			Name:         v.Name,
			Kind:         "variable",
			Code:         v.Code,
			Doc:          v.Doc.Comment,
			Filepath:     v.Filepath,
			Namespace:    v.Namespace.Name,
			Language:     extract.Go,
			Pos:          v.Pos,
			End:          v.End,
			NameVariants: generateNameVariants(v.Name),
		})
	}

	if len(docs) == 0 {
		slog.Info("No documents to ingest into Typesense")
		return nil
	}

	slog.Info("Upserting documents into Typesense", "count", len(docs))
	return i.ts.UpsertDocuments(ctx, docs)
}

func (i *Ingestor) ingestToArangoDB(ctx context.Context, res extract.ExtractNodesResult) error {
	// Ingest nodes by collection type
	if err := i.ingestFunctionNodes(ctx, res.Functions); err != nil {
		return fmt.Errorf("ingest function nodes: %w", err)
	}

	if err := i.ingestTypeNodes(ctx, res.TypeDecls, res.Interfaces, res.NamedTypes); err != nil {
		return fmt.Errorf("ingest type nodes: %w", err)
	}

	if err := i.ingestMemberNodes(ctx, res.Members, res.Vars); err != nil {
		return fmt.Errorf("ingest member nodes: %w", err)
	}

	if err := i.ingestFileNodes(ctx, res.Files); err != nil {
		return fmt.Errorf("ingest file nodes: %w", err)
	}

	if err := i.ingestModuleNodes(ctx, res.Namespaces, res.Files); err != nil {
		return fmt.Errorf("ingest module nodes: %w", err)
	}

	// Ingest edges
	if err := i.ingestCallEdges(ctx, res.Functions); err != nil {
		return fmt.Errorf("ingest call edges: %w", err)
	}

	if err := i.ingestReturnEdges(ctx, res.Functions); err != nil {
		return fmt.Errorf("ingest return edges: %w", err)
	}

	if err := i.ingestParamEdges(ctx, res.Functions); err != nil {
		return fmt.Errorf("ingest param edges: %w", err)
	}

	if err := i.ingestImplementsEdges(ctx, res.TypeDecls); err != nil {
		return fmt.Errorf("ingest implements edges: %w", err)
	}

	if err := i.ingestParentEdges(ctx, res.Functions, res.Members); err != nil {
		return fmt.Errorf("ingest parent edges: %w", err)
	}

	if err := i.ingestImportEdges(ctx, res.Files); err != nil {
		return fmt.Errorf("ingest import edges: %w", err)
	}

	return nil
}

func (i *Ingestor) ingestFunctionNodes(ctx context.Context, functions map[string]extract.Function) error {
	if len(functions) == 0 {
		return nil
	}

	nodes := make([]arangodb.Node, 0, len(functions))
	for qname, fn := range functions {
		if qname == "" || fn.Filepath == "" {
			continue
		}
		isMethod := fn.ParentQName != ""
		nodes = append(nodes, arangodb.Node{
			QName:     qname,
			Name:      fn.Name,
			Kind:      "function", // Always "function" per schema; is_method distinguishes
			Doc:       fn.Doc.Comment,
			Filepath:  fn.Filepath,
			Namespace: fn.Namespace.Name,
			Language:  extract.Go,
			Pos:       fn.Pos,
			End:       fn.End,
			IsMethod:  isMethod,
		})
	}

	slog.Info("Upserting function nodes", "count", len(nodes))
	return i.arango.UpsertNodes(ctx, "functions", nodes)
}

func (i *Ingestor) ingestTypeNodes(ctx context.Context, decls, interfaces map[string]extract.TypeDecl, named map[string]extract.Named) error {
	var nodes []arangodb.Node

	for qname, decl := range decls {
		if qname == "" {
			continue
		}
		nodes = append(nodes, arangodb.Node{
			QName:     qname,
			Name:      decl.Name,
			Kind:      string(decl.Kind),
			Doc:       decl.Doc.Comment,
			Filepath:  decl.Filepath,
			Namespace: decl.Namespace.Name,
			Language:  extract.Go,
			Pos:       decl.Pos,
			End:       decl.End,
		})
	}

	for qname, iface := range interfaces {
		if qname == "" {
			continue
		}
		nodes = append(nodes, arangodb.Node{
			QName:     qname,
			Name:      iface.Name,
			Kind:      "interface",
			Doc:       iface.Doc.Comment,
			Filepath:  iface.Filepath,
			Namespace: iface.Namespace.Name,
			Language:  extract.Go,
			Pos:       iface.Pos,
			End:       iface.End,
		})
	}

	for qname, n := range named {
		if qname == "" {
			continue
		}
		nodes = append(nodes, arangodb.Node{
			QName:     qname,
			Name:      n.Name,
			Kind:      "alias",
			Doc:       n.Doc.Comment,
			Filepath:  n.Filepath,
			Namespace: n.Namespace.Name,
			Language:  extract.Go,
			Pos:       n.Pos,
			End:       n.End,
		})
	}

	if len(nodes) == 0 {
		return nil
	}

	slog.Info("Upserting type nodes", "count", len(nodes))
	return i.arango.UpsertNodes(ctx, "types", nodes)
}

func (i *Ingestor) ingestMemberNodes(ctx context.Context, members map[string]extract.Member, vars map[string]extract.Variable) error {
	var nodes []arangodb.Node

	for qname, member := range members {
		if qname == "" {
			continue
		}
		nodes = append(nodes, arangodb.Node{
			QName:     qname,
			Name:      member.Name,
			Kind:      "member",
			Doc:       member.Doc.Comment,
			Filepath:  member.Filepath,
			Namespace: member.Namespace.Name,
			Language:  extract.Go,
			Pos:       member.Pos,
			End:       member.End,
			TypeQName: member.TypeQName,
		})
	}

	for qname, v := range vars {
		if qname == "" {
			continue
		}
		nodes = append(nodes, arangodb.Node{
			QName:     qname,
			Name:      v.Name,
			Kind:      "variable",
			Doc:       v.Doc.Comment,
			Filepath:  v.Filepath,
			Namespace: v.Namespace.Name,
			Language:  extract.Go,
			Pos:       v.Pos,
			End:       v.End,
			TypeQName: v.TypeQName,
		})
	}

	if len(nodes) == 0 {
		return nil
	}

	slog.Info("Upserting member nodes", "count", len(nodes))
	return i.arango.UpsertNodes(ctx, "members", nodes)
}

func (i *Ingestor) ingestFileNodes(ctx context.Context, files map[string]extract.File) error {
	if len(files) == 0 {
		return nil
	}

	nodes := make([]arangodb.Node, 0, len(files))
	for filename, file := range files {
		if filename == "" {
			continue
		}
		nodes = append(nodes, arangodb.Node{
			QName:     filename,
			Name:      filename,
			Kind:      "file",
			Namespace: file.Namespace.Name,
			Language:  file.Language,
		})
	}

	slog.Info("Upserting file nodes", "count", len(nodes))
	return i.arango.UpsertNodes(ctx, "files", nodes)
}

func (i *Ingestor) ingestModuleNodes(ctx context.Context, namespaces []extract.Namespace, files map[string]extract.File) error {
	moduleSet := make(map[string]bool)

	for _, ns := range namespaces {
		if ns.Name != "" {
			moduleSet[ns.Name] = true
		}
	}

	// Also collect import paths as modules
	for _, file := range files {
		for _, imp := range file.Imports {
			if imp.Path != "" {
				moduleSet[imp.Path] = true
			}
		}
	}

	if len(moduleSet) == 0 {
		return nil
	}

	nodes := make([]arangodb.Node, 0, len(moduleSet))
	for module := range moduleSet {
		nodes = append(nodes, arangodb.Node{
			QName:    module,
			Name:     module,
			Kind:     "module",
			Language: extract.Go,
		})
	}

	slog.Info("Upserting module nodes", "count", len(nodes))
	return i.arango.UpsertNodes(ctx, "modules", nodes)
}

func (i *Ingestor) ingestCallEdges(ctx context.Context, functions map[string]extract.Function) error {
	var edges []arangodb.Edge

	for _, fn := range functions {
		if fn.QName == "" {
			continue
		}
		for _, callee := range fn.Calls {
			if callee == "" {
				continue
			}
			edges = append(edges, arangodb.Edge{
				From:     fn.QName,
				To:       callee,
				FromKind: kindForFunction(fn),
				ToKind:   "function",
			})
		}
	}

	if len(edges) == 0 {
		return nil
	}

	slog.Info("Upserting call edges", "count", len(edges))
	return i.arango.UpsertEdges(ctx, "calls", edges)
}

func (i *Ingestor) ingestReturnEdges(ctx context.Context, functions map[string]extract.Function) error {
	var edges []arangodb.Edge

	for _, fn := range functions {
		if fn.QName == "" {
			continue
		}
		for _, ret := range fn.ReturnQNames {
			if ret == "" {
				continue
			}
			edges = append(edges, arangodb.Edge{
				From:     fn.QName,
				To:       ret,
				FromKind: kindForFunction(fn),
				ToKind:   "struct", // Return types are typically structs/interfaces
			})
		}
	}

	if len(edges) == 0 {
		return nil
	}

	slog.Info("Upserting return edges", "count", len(edges))
	return i.arango.UpsertEdges(ctx, "returns", edges)
}

func (i *Ingestor) ingestParamEdges(ctx context.Context, functions map[string]extract.Function) error {
	var edges []arangodb.Edge

	for _, fn := range functions {
		if fn.QName == "" {
			continue
		}
		for _, param := range fn.ParamQNames {
			if param == "" {
				continue
			}
			edges = append(edges, arangodb.Edge{
				From:     param,
				To:       fn.QName,
				FromKind: "struct", // Param types are typically structs/interfaces
				ToKind:   kindForFunction(fn),
			})
		}
	}

	if len(edges) == 0 {
		return nil
	}

	slog.Info("Upserting param edges", "count", len(edges))
	return i.arango.UpsertEdges(ctx, "param_of", edges)
}

func (i *Ingestor) ingestImplementsEdges(ctx context.Context, decls map[string]extract.TypeDecl) error {
	var edges []arangodb.Edge

	for qname, decl := range decls {
		if qname == "" {
			continue
		}
		for _, iface := range decl.ImplementsQName {
			if iface == "" {
				continue
			}
			edges = append(edges, arangodb.Edge{
				From:     qname,
				To:       iface,
				FromKind: string(decl.Kind),
				ToKind:   "interface",
			})
		}
	}

	if len(edges) == 0 {
		return nil
	}

	slog.Info("Upserting implements edges", "count", len(edges))
	return i.arango.UpsertEdges(ctx, "implements", edges)
}

func (i *Ingestor) ingestParentEdges(ctx context.Context, functions map[string]extract.Function, members map[string]extract.Member) error {
	var edges []arangodb.Edge

	// Methods have parent types
	for _, fn := range functions {
		if fn.QName == "" || fn.ParentQName == "" {
			continue
		}
		edges = append(edges, arangodb.Edge{
			From:     fn.QName,
			To:       fn.ParentQName,
			FromKind: "method",
			ToKind:   "struct",
		})
	}

	// Members have parent types
	for _, member := range members {
		if member.QName == "" || member.ParentQName == "" {
			continue
		}
		edges = append(edges, arangodb.Edge{
			From:     member.QName,
			To:       member.ParentQName,
			FromKind: "member",
			ToKind:   "struct",
		})
	}

	if len(edges) == 0 {
		return nil
	}

	slog.Info("Upserting parent edges", "count", len(edges))
	return i.arango.UpsertEdges(ctx, "parent", edges)
}

func (i *Ingestor) ingestImportEdges(ctx context.Context, files map[string]extract.File) error {
	var edges []arangodb.Edge

	for filename, file := range files {
		if filename == "" {
			continue
		}
		for _, imp := range file.Imports {
			if imp.Path == "" {
				continue
			}
			edges = append(edges, arangodb.Edge{
				From:     filename,
				To:       imp.Path,
				FromKind: "file",
				ToKind:   "module",
			})
		}
	}

	if len(edges) == 0 {
		return nil
	}

	slog.Info("Upserting import edges", "count", len(edges))
	return i.arango.UpsertEdges(ctx, "imports", edges)
}

// kindForFunction returns "method" if the function has a receiver, otherwise "function".
func kindForFunction(fn extract.Function) string {
	if fn.ParentQName != "" {
		return "method"
	}
	return "function"
}

// generateNameVariants creates searchable variations of a name.
// For example, "ProcessPayment" becomes ["ProcessPayment", "process_payment", "processpayment"].
func generateNameVariants(name string) []string {
	if name == "" {
		return nil
	}

	variants := []string{name}
	seen := map[string]bool{name: true}

	// Add lowercase
	lower := strings.ToLower(name)
	if !seen[lower] {
		variants = append(variants, lower)
		seen[lower] = true
	}

	// Convert CamelCase to snake_case
	snake := camelToSnake(name)
	if !seen[snake] {
		variants = append(variants, snake)
		seen[snake] = true
	}

	// Convert CamelCase to kebab-case
	kebab := strings.ReplaceAll(snake, "_", "-")
	if !seen[kebab] {
		variants = append(variants, kebab)
		seen[kebab] = true
	}

	// Split CamelCase into words
	words := splitCamelCase(name)
	for _, word := range words {
		wordLower := strings.ToLower(word)
		if !seen[wordLower] && len(wordLower) > 2 {
			variants = append(variants, wordLower)
			seen[wordLower] = true
		}
	}

	return variants
}

var camelCaseRe = regexp.MustCompile(`([a-z0-9])([A-Z])`)

func camelToSnake(s string) string {
	snake := camelCaseRe.ReplaceAllString(s, "${1}_${2}")
	return strings.ToLower(snake)
}

func splitCamelCase(s string) []string {
	var words []string
	var current strings.Builder

	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}
