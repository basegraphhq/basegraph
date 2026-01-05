package arangodb

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/arangodb/go-driver/v2/arangodb"
	"github.com/arangodb/go-driver/v2/connection"
)

var ErrNotFound = errors.New("document not found")

type Client interface {
	// Setup operations
	EnsureDatabase(ctx context.Context) error
	EnsureCollections(ctx context.Context) error
	EnsureGraph(ctx context.Context) error

	// Write operations (for ingestion)
	IngestNodes(ctx context.Context, collection string, nodes []Node) error
	IngestEdges(ctx context.Context, collection string, edges []Edge) error
	TruncateCollections(ctx context.Context) error

	// Read operations (for explore agent)
	GetCallers(ctx context.Context, qname string, depth int) ([]GraphNode, error)
	GetCallees(ctx context.Context, qname string, depth int) ([]GraphNode, error)
	GetChildren(ctx context.Context, qname string) ([]GraphNode, error)
	GetImplementations(ctx context.Context, qname string) ([]GraphNode, error)
	GetMethods(ctx context.Context, qname string) ([]GraphNode, error)
	GetUsages(ctx context.Context, qname string) ([]GraphNode, error)
	GetInheritors(ctx context.Context, qname string) ([]GraphNode, error)
	TraverseFrom(ctx context.Context, qnames []string, opts TraversalOptions) ([]GraphNode, []GraphEdge, error)

	// Symbol discovery operations
	GetFileSymbols(ctx context.Context, opts FileSymbolsOptions) ([]FileSymbol, error)
	SearchSymbols(ctx context.Context, opts SearchOptions) ([]SearchResult, int, error) // returns results, total count, error
	ResolveSymbol(ctx context.Context, opts SearchOptions) (ResolvedSymbol, error)      // returns single symbol or error

	// Utility
	Close() error
}

type Config struct {
	URL      string
	Username string
	Password string
	Database string
}

func (c Config) Validate() error {
	if c.URL == "" {
		return fmt.Errorf("arangodb URL is required")
	}
	if c.Username == "" {
		return fmt.Errorf("arangodb username is required")
	}
	if c.Database == "" {
		return fmt.Errorf("arangodb database name is required")
	}
	return nil
}

type client struct {
	conn         connection.Connection
	arangoClient arangodb.Client
	db           arangodb.Database
	cfg          Config
}

func New(ctx context.Context, cfg Config) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("arangodb config: %w", err)
	}

	endpoint := connection.NewRoundRobinEndpoints([]string{cfg.URL}) // round robins from the urls. we just have one for now
	conn := connection.NewHttp2Connection(connection.DefaultHTTP2ConfigurationWrapper(endpoint, true))

	auth := connection.NewBasicAuth(cfg.Username, cfg.Password)
	if err := conn.SetAuthentication(auth); err != nil {
		return nil, fmt.Errorf("arangodb auth: %w", err)
	}

	arangoClient := arangodb.NewClient(conn)

	c := &client{
		conn:         conn,
		arangoClient: arangoClient,
		cfg:          cfg,
	}

	return c, nil
}

func (c *client) Close() error {
	return nil
}

func (c *client) EnsureDatabase(ctx context.Context) error {
	start := time.Now()

	exists, err := c.arangoClient.DatabaseExists(ctx, c.cfg.Database)
	if err != nil {
		return fmt.Errorf("check database exists: %w", err)
	}

	if !exists {
		_, err = c.arangoClient.CreateDatabase(ctx, c.cfg.Database, nil)
		if err != nil {
			return fmt.Errorf("create database: %w", err)
		}
		slog.InfoContext(ctx, "arangodb database created",
			"database", c.cfg.Database,
			"duration_ms", time.Since(start).Milliseconds())
	}

	db, err := c.arangoClient.GetDatabase(ctx, c.cfg.Database, nil)
	if err != nil {
		return fmt.Errorf("get database: %w", err)
	}
	c.db = db

	return nil
}

func (c *client) EnsureCollections(ctx context.Context) error {
	if c.db == nil {
		return fmt.Errorf("database not initialized, call EnsureDatabase first")
	}

	nodeCollections := []string{"functions", "types", "members", "files", "modules"}
	edgeCollections := []string{"calls", "implements", "inherits", "returns", "param_of", "parent", "imports", "decorated_by"}

	for _, name := range nodeCollections {
		if err := c.ensureCollection(ctx, name, false); err != nil {
			return err
		}
	}

	for _, name := range edgeCollections {
		if err := c.ensureCollection(ctx, name, true); err != nil {
			return err
		}
	}

	// Ensure indexes for symbol discovery queries
	if err := c.ensureIndexes(ctx); err != nil {
		return fmt.Errorf("ensure indexes: %w", err)
	}

	return nil
}

// ensureIndexes creates indexes for efficient symbol discovery queries.
func (c *client) ensureIndexes(ctx context.Context) error {
	// Index on 'filepath' for GetFileSymbols query
	// Index on 'name' for SearchSymbols query
	indexedCollections := []string{"functions", "types", "members"}

	for _, colName := range indexedCollections {
		col, err := c.db.GetCollection(ctx, colName, nil)
		if err != nil {
			return fmt.Errorf("get collection %s: %w", colName, err)
		}

		// Filepath index - for symbols(file) operation
		_, isNew, err := col.EnsurePersistentIndex(ctx, []string{"filepath"}, &arangodb.CreatePersistentIndexOptions{
			Name: "idx_filepath",
		})
		if err != nil {
			return fmt.Errorf("ensure filepath index on %s: %w", colName, err)
		}
		if isNew {
			slog.InfoContext(ctx, "arangodb index created", "collection", colName, "index", "idx_filepath")
		}

		// Name index - for search(name) operation
		_, isNew, err = col.EnsurePersistentIndex(ctx, []string{"name"}, &arangodb.CreatePersistentIndexOptions{
			Name: "idx_name",
		})
		if err != nil {
			return fmt.Errorf("ensure name index on %s: %w", colName, err)
		}
		if isNew {
			slog.InfoContext(ctx, "arangodb index created", "collection", colName, "index", "idx_name")
		}
	}

	return nil
}

func (c *client) ensureCollection(ctx context.Context, name string, isEdge bool) error {
	exists, err := c.db.CollectionExists(ctx, name)
	if err != nil {
		return fmt.Errorf("check collection %s exists: %w", name, err)
	}

	if !exists {
		props := &arangodb.CreateCollectionPropertiesV2{}
		if isEdge {
			colType := arangodb.CollectionTypeEdge
			props.Type = &colType
		} else {
			colType := arangodb.CollectionTypeDocument
			props.Type = &colType
		}

		_, err = c.db.CreateCollectionV2(ctx, name, props)
		if err != nil {
			return fmt.Errorf("create collection %s: %w", name, err)
		}
		slog.InfoContext(ctx, "arangodb collection created",
			"collection", name,
			"is_edge", isEdge)
	}

	return nil
}

func (c *client) EnsureGraph(ctx context.Context) error {
	if c.db == nil {
		return fmt.Errorf("database not initialized, call EnsureDatabase first")
	}

	graphName := "codegraph"
	exists, err := c.db.GraphExists(ctx, graphName)
	if err != nil {
		return fmt.Errorf("check graph exists: %w", err)
	}

	if exists {
		return nil
	}

	graphDef := &arangodb.GraphDefinition{
		Name: graphName,
		EdgeDefinitions: []arangodb.EdgeDefinition{
			{Collection: "calls", From: []string{"functions"}, To: []string{"functions"}},
			{Collection: "implements", From: []string{"types"}, To: []string{"types"}},
			{Collection: "inherits", From: []string{"types"}, To: []string{"types"}},
			{Collection: "returns", From: []string{"functions"}, To: []string{"types"}},
			{Collection: "param_of", From: []string{"types"}, To: []string{"functions"}},
			{Collection: "parent", From: []string{"functions", "members"}, To: []string{"types", "files"}},
			{Collection: "imports", From: []string{"files"}, To: []string{"modules"}},
			{Collection: "decorated_by", From: []string{"functions", "types"}, To: []string{"functions"}},
		},
	}

	_, err = c.db.CreateGraph(ctx, graphName, graphDef, nil)
	if err != nil {
		return fmt.Errorf("create graph: %w", err)
	}

	slog.InfoContext(ctx, "arangodb graph created", "graph", graphName)
	return nil
}

func (c *client) TruncateCollections(ctx context.Context) error {
	if c.db == nil {
		return fmt.Errorf("database not initialized")
	}

	start := time.Now()

	nodeCollections := []string{"functions", "types", "members", "files", "modules"}
	edgeCollections := []string{"calls", "implements", "inherits", "returns", "param_of", "parent", "imports", "decorated_by"}

	allCollections := append(nodeCollections, edgeCollections...)

	for _, name := range allCollections {
		col, err := c.db.GetCollection(ctx, name, nil)
		if err != nil {
			return fmt.Errorf("get collection %s: %w", name, err)
		}

		if err := col.Truncate(ctx); err != nil {
			return fmt.Errorf("truncate collection %s: %w", name, err)
		}
	}

	slog.InfoContext(ctx, "arangodb collections truncated",
		"collections", len(allCollections),
		"duration_ms", time.Since(start).Milliseconds())

	return nil
}

// IngestNodes inserts new node documents into the specified collection.
// Duplicates (same _key) are silently ignored - existing documents are NOT updated.
// For MVP: use TruncateCollections before ingesting to ensure a clean rebuild.
func (c *client) IngestNodes(ctx context.Context, collection string, nodes []Node) error {
	if c.db == nil {
		return fmt.Errorf("database not initialized")
	}

	if len(nodes) == 0 {
		return nil
	}

	start := time.Now()
	col, err := c.db.GetCollection(ctx, collection, nil)
	if err != nil {
		return fmt.Errorf("get collection %s: %w", collection, err)
	}

	docs := make([]map[string]any, len(nodes))
	for i, node := range nodes {
		doc := map[string]any{
			"_key":      makeKey(node.QName),
			"qname":     node.QName,
			"name":      node.Name,
			"kind":      node.Kind,
			"doc":       node.Doc,
			"filepath":  node.Filepath,
			"namespace": node.Namespace,
			"language":  node.Language,
			"pos":       node.Pos,
			"end":       node.End,
		}
		// Add optional fields based on node type
		if node.IsMethod {
			doc["is_method"] = true
		}
		if node.TypeQName != "" {
			doc["type_qname"] = node.TypeQName
		}
		if node.Signature != "" {
			doc["signature"] = node.Signature
		}
		docs[i] = doc
	}

	reader, err := col.CreateDocuments(ctx, docs)
	if err != nil {
		return fmt.Errorf("create documents: %w", err)
	}

	// Consume all responses (ignoring errors for duplicate keys)
	for {
		_, readErr := reader.Read()
		if readErr != nil {
			break
		}
	}

	slog.DebugContext(ctx, "arangodb nodes ingested",
		"collection", collection,
		"count", len(nodes),
		"duration_ms", time.Since(start).Milliseconds())

	return nil
}

// IngestEdges inserts new edge documents into the specified collection.
// Duplicates (same _key) are silently ignored - existing documents are NOT updated.
// For MVP: use TruncateCollections before ingesting to ensure a clean rebuild.
func (c *client) IngestEdges(ctx context.Context, collection string, edges []Edge) error {
	if c.db == nil {
		return fmt.Errorf("database not initialized")
	}

	if len(edges) == 0 {
		return nil
	}

	start := time.Now()
	col, err := c.db.GetCollection(ctx, collection, nil)
	if err != nil {
		return fmt.Errorf("get collection %s: %w", collection, err)
	}

	docs := make([]map[string]any, len(edges))
	for i, edge := range edges {
		fromCol := nodeCollectionForKind(edge.FromKind)
		toCol := nodeCollectionForKind(edge.ToKind)

		docs[i] = map[string]any{
			"_key":  makeEdgeKey(edge.From, edge.To),
			"_from": fmt.Sprintf("%s/%s", fromCol, makeKey(edge.From)),
			"_to":   fmt.Sprintf("%s/%s", toCol, makeKey(edge.To)),
		}

		for k, v := range edge.Properties {
			docs[i][k] = v
		}
	}

	reader, err := col.CreateDocuments(ctx, docs)
	if err != nil {
		return fmt.Errorf("create edge documents: %w", err)
	}

	// Consume all responses (ignoring errors for duplicate keys)
	for {
		_, readErr := reader.Read()
		if readErr != nil {
			break
		}
	}

	slog.DebugContext(ctx, "arangodb edges ingested",
		"collection", collection,
		"count", len(edges),
		"duration_ms", time.Since(start).Milliseconds())

	return nil
}

func (c *client) GetCallers(ctx context.Context, qname string, depth int) ([]GraphNode, error) {
	if depth <= 0 {
		depth = 1
	}

	query := `
		FOR v IN 1..@depth INBOUND @start GRAPH "codegraph"
			OPTIONS { edgeCollections: ["calls"] }
			LIMIT 30
			RETURN { qname: v.qname, name: v.name, kind: v.kind, filepath: v.filepath, pos: v.pos, signature: v.signature }
	`

	return c.executeTraversal(ctx, query, qname, depth)
}

func (c *client) GetCallees(ctx context.Context, qname string, depth int) ([]GraphNode, error) {
	if depth <= 0 {
		depth = 1
	}

	query := `
		FOR v IN 1..@depth OUTBOUND @start GRAPH "codegraph"
			OPTIONS { edgeCollections: ["calls"] }
			LIMIT 30
			RETURN { qname: v.qname, name: v.name, kind: v.kind, filepath: v.filepath, pos: v.pos, signature: v.signature }
	`

	return c.executeTraversal(ctx, query, qname, depth)
}

func (c *client) GetChildren(ctx context.Context, qname string) ([]GraphNode, error) {
	query := `
		FOR v IN 1..1 INBOUND @start GRAPH "codegraph"
			OPTIONS { edgeCollections: ["parent"] }
			RETURN { qname: v.qname, name: v.name, kind: v.kind, filepath: v.filepath, pos: v.pos, signature: v.signature }
	`

	return c.executeTraversalFrom(ctx, query, "types", qname, 1)
}

func (c *client) GetImplementations(ctx context.Context, qname string) ([]GraphNode, error) {
	query := `
		FOR v IN 1..1 INBOUND @start GRAPH "codegraph"
			OPTIONS { edgeCollections: ["implements"] }
			RETURN { qname: v.qname, name: v.name, kind: v.kind, filepath: v.filepath, pos: v.pos, signature: v.signature }
	`

	return c.executeTraversalFrom(ctx, query, "types", qname, 1)
}

func (c *client) GetMethods(ctx context.Context, qname string) ([]GraphNode, error) {
	query := `
		FOR v IN 1..1 INBOUND @start GRAPH "codegraph"
			OPTIONS { edgeCollections: ["parent"] }
			RETURN { qname: v.qname, name: v.name, kind: v.kind, filepath: v.filepath, pos: v.pos, signature: v.signature }
	`

	return c.executeTraversalFrom(ctx, query, "types", qname, 1)
}

func (c *client) GetUsages(ctx context.Context, qname string) ([]GraphNode, error) {
	query := `
		FOR v IN 1..1 INBOUND @start GRAPH "codegraph"
			OPTIONS { edgeCollections: ["param_of", "returns"] }
			RETURN { qname: v.qname, name: v.name, kind: v.kind, filepath: v.filepath, pos: v.pos, signature: v.signature }
	`

	return c.executeTraversalFrom(ctx, query, "types", qname, 1)
}

func (c *client) GetInheritors(ctx context.Context, qname string) ([]GraphNode, error) {
	query := `
		FOR v IN 1..1 INBOUND @start GRAPH "codegraph"
			OPTIONS { edgeCollections: ["inherits"] }
			RETURN { qname: v.qname, name: v.name, kind: v.kind, filepath: v.filepath, pos: v.pos, signature: v.signature }
	`

	return c.executeTraversalFrom(ctx, query, "types", qname, 1)
}

func (c *client) executeTraversal(ctx context.Context, query string, qname string, depth int) ([]GraphNode, error) {
	return c.executeTraversalFrom(ctx, query, "functions", qname, depth)
}

func (c *client) executeTraversalFrom(ctx context.Context, query string, collection string, qname string, depth int) ([]GraphNode, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	start := time.Now()

	startVertex := fmt.Sprintf("%s/%s", collection, makeKey(qname))

	bindVars := map[string]any{
		"start": startVertex,
	}
	// Only add depth if the query uses it
	if strings.Contains(query, "@depth") {
		bindVars["depth"] = depth
	}

	cursor, err := c.db.Query(ctx, query, &arangodb.QueryOptions{
		BindVars: bindVars,
	})
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer cursor.Close()

	var results []GraphNode
	for cursor.HasMore() {
		var doc struct {
			QName     string `json:"qname"`
			Name      string `json:"name"`
			Kind      string `json:"kind"`
			Filepath  string `json:"filepath"`
			Pos       int    `json:"pos"`
			Signature string `json:"signature"`
		}
		_, err := cursor.ReadDocument(ctx, &doc)
		if err != nil {
			return nil, fmt.Errorf("read document: %w", err)
		}
		// Skip nodes that weren't found (external/stdlib references)
		if doc.QName == "" {
			continue
		}
		results = append(results, GraphNode{
			QName:     doc.QName,
			Name:      doc.Name,
			Kind:      doc.Kind,
			Filepath:  doc.Filepath,
			Pos:       doc.Pos,
			Signature: doc.Signature,
		})
	}

	slog.DebugContext(ctx, "arangodb traversal completed",
		"qname", qname,
		"depth", depth,
		"results", len(results),
		"duration_ms", time.Since(start).Milliseconds())

	return results, nil
}

func (c *client) TraverseFrom(ctx context.Context, qnames []string, opts TraversalOptions) ([]GraphNode, []GraphEdge, error) {
	if c.db == nil {
		return nil, nil, fmt.Errorf("database not initialized")
	}

	if len(qnames) == 0 {
		return nil, nil, nil
	}

	start := time.Now()

	direction := "OUTBOUND"
	switch opts.Direction {
	case DirectionInbound:
		direction = "INBOUND"
	case DirectionAny:
		direction = "ANY"
	}

	depth := opts.MaxDepth
	if depth <= 0 {
		depth = 2
	}

	edgeFilter := ""
	if len(opts.EdgeTypes) > 0 {
		edgeFilter = fmt.Sprintf("OPTIONS { edgeCollections: %v }", opts.EdgeTypes)
	}

	startVertices := make([]string, len(qnames))
	for i, qname := range qnames {
		startVertices[i] = fmt.Sprintf("functions/%s", makeKey(qname))
	}

	query := fmt.Sprintf(`
		FOR startV IN @starts
			FOR v, e IN 1..@depth %s startV GRAPH "codegraph" %s
				RETURN { vertex: { qname: v.qname, name: v.name, kind: v.kind }, edge: e }
	`, direction, edgeFilter)

	cursor, err := c.db.Query(ctx, query, &arangodb.QueryOptions{
		BindVars: map[string]any{
			"starts": startVertices,
			"depth":  depth,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("execute traversal: %w", err)
	}
	defer cursor.Close()

	nodeMap := make(map[string]GraphNode)
	var edges []GraphEdge

	for cursor.HasMore() {
		var doc struct {
			Vertex struct {
				QName string `json:"qname"`
				Name  string `json:"name"`
				Kind  string `json:"kind"`
			} `json:"vertex"`
			Edge map[string]any `json:"edge"`
		}
		_, err := cursor.ReadDocument(ctx, &doc)
		if err != nil {
			return nil, nil, fmt.Errorf("read document: %w", err)
		}

		if doc.Vertex.QName != "" {
			nodeMap[doc.Vertex.QName] = GraphNode{
				QName: doc.Vertex.QName,
				Name:  doc.Vertex.Name,
				Kind:  doc.Vertex.Kind,
			}
		}

		if doc.Edge != nil {
			from, _ := doc.Edge["_from"].(string)
			to, _ := doc.Edge["_to"].(string)
			edges = append(edges, GraphEdge{
				From: extractQNameFromID(from),
				To:   extractQNameFromID(to),
			})
		}
	}

	nodes := make([]GraphNode, 0, len(nodeMap))
	for _, node := range nodeMap {
		nodes = append(nodes, node)
	}

	slog.DebugContext(ctx, "arangodb multi-traversal completed",
		"start_count", len(qnames),
		"depth", depth,
		"nodes", len(nodes),
		"edges", len(edges),
		"duration_ms", time.Since(start).Milliseconds())

	return nodes, edges, nil
}

func makeKey(qname string) string {
	hash := md5.Sum([]byte(qname))
	return hex.EncodeToString(hash[:])[:16]
}

func makeEdgeKey(from, to string) string {
	combined := from + "->" + to
	hash := md5.Sum([]byte(combined))
	return hex.EncodeToString(hash[:])[:16]
}

func nodeCollectionForKind(kind string) string {
	switch kind {
	case "function", "method":
		return "functions"
	case "struct", "class", "interface", "alias":
		return "types"
	case "field", "member", "variable":
		return "members"
	case "file":
		return "files"
	case "module", "package", "namespace":
		return "modules"
	default:
		return "functions"
	}
}

func extractQNameFromID(id string) string {
	return id
}

// GetFileSymbols returns all symbols defined in a file, sorted by position.
// If opts.Kind is set, only symbols of that kind are returned.
func (c *client) GetFileSymbols(ctx context.Context, opts FileSymbolsOptions) ([]FileSymbol, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	start := time.Now()
	filepath := opts.Filepath

	// Build kind filter clause
	// Note: "method" is stored as kind="function" + is_method=true
	kindFilter := ""
	switch opts.Kind {
	case "method":
		kindFilter = "FILTER doc.is_method == true"
	case "function":
		kindFilter = "FILTER doc.kind == 'function' AND (doc.is_method == null OR doc.is_method == false)"
	case "struct", "interface", "class", "type":
		kindFilter = "FILTER doc.kind == @kind"
	case "field", "const", "var":
		kindFilter = "FILTER doc.kind == @kind"
	case "":
		// No filter
	default:
		kindFilter = "FILTER doc.kind == @kind"
	}

	// Query all collections for symbols in this file
	// Note: is_method=true means it's a method, so we return "method" as kind for display
	// Use suffix matching to handle relative vs absolute paths
	query := fmt.Sprintf(`
		FOR doc IN UNION(
			(FOR f IN functions FILTER f.filepath == @filepath OR f.filepath LIKE @pathPattern RETURN f),
			(FOR t IN types FILTER t.filepath == @filepath OR t.filepath LIKE @pathPattern RETURN t),
			(FOR m IN members FILTER m.filepath == @filepath OR m.filepath LIKE @pathPattern RETURN m)
		)
		%s
		SORT doc.pos ASC
		RETURN { 
			qname: doc.qname, 
			name: doc.name, 
			kind: doc.is_method ? "method" : doc.kind, 
			signature: doc.signature,
			pos: doc.pos, 
			end: doc.end 
		}
	`, kindFilter)

	// Create suffix pattern for matching: "%" + "/path/to/file.go"
	pathPattern := "%" + filepath
	if strings.HasPrefix(filepath, "/") {
		// Already absolute, just use exact match (pathPattern won't match anything extra)
		pathPattern = filepath
	}

	bindVars := map[string]any{
		"filepath":    filepath,
		"pathPattern": pathPattern,
	}
	if opts.Kind != "" && opts.Kind != "method" && opts.Kind != "function" {
		bindVars["kind"] = opts.Kind
	}

	cursor, err := c.db.Query(ctx, query, &arangodb.QueryOptions{
		BindVars: bindVars,
	})
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer cursor.Close()

	var results []FileSymbol
	for cursor.HasMore() {
		var doc struct {
			QName     string `json:"qname"`
			Name      string `json:"name"`
			Kind      string `json:"kind"`
			Signature string `json:"signature"`
			Pos       int    `json:"pos"`
			End       int    `json:"end"`
		}
		_, err := cursor.ReadDocument(ctx, &doc)
		if err != nil {
			return nil, fmt.Errorf("read document: %w", err)
		}
		results = append(results, FileSymbol{
			QName:     doc.QName,
			Name:      doc.Name,
			Kind:      doc.Kind,
			Signature: doc.Signature,
			Pos:       doc.Pos,
			End:       doc.End,
		})
	}

	slog.DebugContext(ctx, "arangodb file symbols retrieved",
		"filepath", opts.Filepath,
		"kind", opts.Kind,
		"count", len(results),
		"duration_ms", time.Since(start).Milliseconds())

	return results, nil
}

// SearchSymbols finds symbols by name pattern with optional filters.
// Returns matching symbols, total count, and error.
func (c *client) SearchSymbols(ctx context.Context, opts SearchOptions) ([]SearchResult, int, error) {
	if c.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	start := time.Now()

	// Convert glob pattern to AQL LIKE pattern: * -> %
	pattern := globToLike(opts.Name)

	// Build dynamic filter clauses
	var filters []string
	bindVars := map[string]any{
		"pattern": pattern,
	}

	// Always filter by name pattern
	filters = append(filters, "LIKE(doc.name, @pattern, true)")

	// Handle kind filter - "method" is stored as kind="function" with is_method=true
	if opts.Kind != "" {
		switch opts.Kind {
		case "method":
			filters = append(filters, "(doc.kind == 'function' AND doc.is_method == true)")
		case "function":
			filters = append(filters, "(doc.kind == 'function' AND (doc.is_method == null OR doc.is_method == false))")
		default:
			filters = append(filters, "doc.kind == @kind")
			bindVars["kind"] = opts.Kind
		}
	}
	if opts.File != "" {
		// Use suffix matching to handle relative vs absolute paths
		if strings.HasPrefix(opts.File, "/") {
			// Absolute path - exact match
			filters = append(filters, "doc.filepath == @file")
			bindVars["file"] = opts.File
		} else {
			// Relative path - match suffix
			filters = append(filters, "(doc.filepath == @file OR doc.filepath LIKE @filePattern)")
			bindVars["file"] = opts.File
			bindVars["filePattern"] = "%" + opts.File
		}
	}
	if opts.Namespace != "" {
		filters = append(filters, "doc.namespace == @namespace")
		bindVars["namespace"] = opts.Namespace
	}

	filterClause := strings.Join(filters, " AND ")

	// Query with limit, but also get total count
	// Note: is_method=true means it's a method, so we return "method" as kind for display
	query := fmt.Sprintf(`
		LET all_results = (
			FOR doc IN UNION(
				(FOR f IN functions RETURN f),
				(FOR t IN types RETURN t),
				(FOR m IN members RETURN m)
			)
			FILTER %s
			RETURN doc
		)
		LET total = LENGTH(all_results)
		LET limited = (
			FOR doc IN all_results
			SORT doc.filepath, doc.pos
			LIMIT 30
			RETURN {
				qname: doc.qname,
				name: doc.name,
				kind: doc.is_method ? "method" : doc.kind,
				signature: doc.signature,
				filepath: doc.filepath,
				pos: doc.pos
			}
		)
		RETURN { results: limited, total: total }
	`, filterClause)

	cursor, err := c.db.Query(ctx, query, &arangodb.QueryOptions{
		BindVars: bindVars,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("execute query: %w", err)
	}
	defer cursor.Close()

	var response struct {
		Results []struct {
			QName     string `json:"qname"`
			Name      string `json:"name"`
			Kind      string `json:"kind"`
			Signature string `json:"signature"`
			Filepath  string `json:"filepath"`
			Pos       int    `json:"pos"`
		} `json:"results"`
		Total int `json:"total"`
	}

	if cursor.HasMore() {
		_, err := cursor.ReadDocument(ctx, &response)
		if err != nil {
			return nil, 0, fmt.Errorf("read document: %w", err)
		}
	}

	results := make([]SearchResult, len(response.Results))
	for i, doc := range response.Results {
		results[i] = SearchResult{
			QName:     doc.QName,
			Name:      doc.Name,
			Kind:      doc.Kind,
			Signature: doc.Signature,
			Filepath:  doc.Filepath,
			Pos:       doc.Pos,
		}
	}

	slog.DebugContext(ctx, "arangodb symbol search completed",
		"pattern", opts.Name,
		"kind", opts.Kind,
		"results", len(results),
		"total", response.Total,
		"duration_ms", time.Since(start).Milliseconds())

	return results, response.Total, nil
}

// globToLike converts glob patterns to SQL LIKE patterns.
// * -> % (match any characters)
func globToLike(pattern string) string {
	return strings.ReplaceAll(pattern, "*", "%")
}

// ResolveSymbol finds a single symbol matching the query.
// Returns AmbiguousSymbolError if multiple matches found (with up to 5 candidates).
// Returns ErrNotFound if no matches found.
func (c *client) ResolveSymbol(ctx context.Context, opts SearchOptions) (ResolvedSymbol, error) {
	results, total, err := c.SearchSymbols(ctx, opts)
	if err != nil {
		return ResolvedSymbol{}, fmt.Errorf("search symbols: %w", err)
	}

	if total == 0 {
		return ResolvedSymbol{}, ErrNotFound
	}

	if total == 1 {
		r := results[0]
		return ResolvedSymbol{
			QName:     r.QName,
			Name:      r.Name,
			Kind:      r.Kind,
			Filepath:  r.Filepath,
			Pos:       r.Pos,
			Signature: r.Signature,
		}, nil
	}

	// Multiple matches - return up to 5 candidates for the error message
	candidates := results
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}

	return ResolvedSymbol{}, AmbiguousSymbolError{
		Query:      opts.Name,
		Candidates: candidates,
	}
}
