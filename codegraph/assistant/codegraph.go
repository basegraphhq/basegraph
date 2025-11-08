package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/humanbeeng/lepo/prototypes/codegraph/process"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

const (
	defaultSearchLimit       = 20
	maxSearchLimit           = 100
	defaultRelationshipLimit = 20
	maxRelationshipLimit     = 50
	maxCodeSnippetCharacters = 4000
	maxGrepSnippetRunes      = 240
	grepContextRunes         = 100
)

const defaultQueryTimeout = 5 * time.Second

type codeGraphTools struct {
	driver   neo4j.DriverWithContext
	database string
}

func newCodeGraphTools(ctx context.Context, reg *ToolRegistry, cfg process.Neo4jConfig) (*codeGraphTools, error) {
	driver, err := neo4j.NewDriver(cfg.URI, neo4j.BasicAuth(cfg.Username, cfg.Password, ""))
	if err != nil {
		return nil, fmt.Errorf("create neo4j driver: %w", err)
	}

	verifyCtx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	if err := driver.VerifyConnectivity(verifyCtx); err != nil {
		_ = driver.Close(ctx)
		return nil, fmt.Errorf("verify neo4j connectivity: %w", err)
	}

	tools := &codeGraphTools{
		driver:   driver,
		database: cfg.Database,
	}

	if err := reg.Add(tools.searchDefinition(), tools.handleSearchSymbols); err != nil {
		_ = driver.Close(ctx)
		return nil, err
	}
	if err := reg.Add(tools.detailDefinition(), tools.handleSymbolDetails); err != nil {
		_ = driver.Close(ctx)
		return nil, err
	}
	if err := reg.Add(tools.grepDefinition(), tools.handleGrepNodes); err != nil {
		_ = driver.Close(ctx)
		return nil, err
	}

	reg.AddCloser(func(closeCtx context.Context) error {
		return driver.Close(closeCtx)
	})

	return tools, nil
}

func (c *codeGraphTools) searchDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "search_code_symbols",
		Description: "Search the Neo4j code graph for symbols by name, qualified name, namespace, or kind.",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Case-insensitive substring to match against symbol name or qualified_name.",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "Optional exact namespace (Go package path) filter.",
				},
				"kinds": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"description": "Optional list of node kinds to include (e.g., function, struct, interface, file, import).",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     maxSearchLimit,
					"description": "Maximum number of results to return (default 20, max 100).",
				},
			},
			"required": []string{"query", "namespace", "kinds", "limit"},
		},
	}
}

func (c *codeGraphTools) detailDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "get_symbol_details",
		Description: "Retrieve rich details for a symbol, including code snippet and related graph edges.",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"qualified_name": map[string]any{
					"type":        "string",
					"description": "Fully-qualified symbol identifier (e.g., package.Func or package.Struct.Method).",
				},
				"relationship_limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     maxRelationshipLimit,
					"description": "Maximum number of related nodes to fetch per relationship (default 20).",
				},
				"include_relationships": map[string]any{
					"type":        "boolean",
					"description": "When false, only node attributes are returned (relationships skipped).",
				},
			},
			"required": []string{"qualified_name", "relationship_limit", "include_relationships"},
		},
	}
}

type searchArgs struct {
	Query     string   `json:"query"`
	Namespace string   `json:"namespace"`
	Kinds     []string `json:"kinds"`
	Limit     int      `json:"limit"`
}

type detailArgs struct {
	QualifiedName        string `json:"qualified_name"`
	RelationshipLimit    int    `json:"relationship_limit"`
	IncludeRelationships bool   `json:"include_relationships"`
}

type searchResult struct {
	Name          string `json:"name,omitempty"`
	QualifiedName string `json:"qualified_name,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	File          string `json:"file,omitempty"`
	Doc           string `json:"doc,omitempty"`
}

type symbolDetails struct {
	Name                string               `json:"name,omitempty"`
	QualifiedName       string               `json:"qualified_name"`
	Kind                string               `json:"kind,omitempty"`
	Namespace           string               `json:"namespace,omitempty"`
	File                string               `json:"file,omitempty"`
	Type                string               `json:"type,omitempty"`
	UnderlyingType      string               `json:"underlying_type,omitempty"`
	ParentQualifiedName string               `json:"parent_qualified_name,omitempty"`
	Doc                 string               `json:"doc,omitempty"`
	Code                string               `json:"code,omitempty"`
	Relationships       *symbolRelationships `json:"relationships,omitempty"`
}

type symbolRelationships struct {
	Calls         []relatedSymbol `json:"calls,omitempty"`
	CalledBy      []relatedSymbol `json:"called_by,omitempty"`
	Implements    []relatedSymbol `json:"implements,omitempty"`
	ImplementedBy []relatedSymbol `json:"implemented_by,omitempty"`
	Returns       []relatedSymbol `json:"returns,omitempty"`
	Params        []relatedSymbol `json:"params,omitempty"`
}

type relatedSymbol struct {
	QualifiedName string `json:"qualified_name,omitempty"`
	Name          string `json:"name,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
}

type grepArgs struct {
	Query         string   `json:"query"`
	Namespace     string   `json:"namespace"`
	Kinds         []string `json:"kinds"`
	Limit         int      `json:"limit"`
	CaseSensitive bool     `json:"case_sensitive"`
	Fields        []string `json:"fields"`
}

type grepResult struct {
	QualifiedName string `json:"qualified_name"`
	Name          string `json:"name,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	File          string `json:"file,omitempty"`
	Field         string `json:"field"`
	Snippet       string `json:"snippet"`
}

func (c *codeGraphTools) handleSearchSymbols(ctx context.Context, raw json.RawMessage) (string, error) {
	var args searchArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", errors.New("query must not be empty")
	}
	limit := args.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	kinds := make([]string, 0, len(args.Kinds))
	for _, k := range args.Kinds {
		trimmed := strings.TrimSpace(strings.ToLower(k))
		if trimmed != "" {
			kinds = append(kinds, trimmed)
		}
	}

	params := map[string]any{
		"query":     strings.ToLower(query),
		"limit":     limit,
		"namespace": strings.TrimSpace(args.Namespace),
		"kinds":     kinds,
	}

	cypher := `
MATCH (n:Node)
WHERE (toLower(n.name) CONTAINS $query OR toLower(n.qualified_name) CONTAINS $query)
  AND ($namespace = "" OR n.namespace = $namespace)
  AND (size($kinds) = 0 OR toLower(n.kind) IN $kinds)
RETURN n.name AS name,
       n.qualified_name AS qualified_name,
       n.kind AS kind,
       n.namespace AS namespace,
       n.file AS file,
       n.doc AS doc
ORDER BY n.kind, n.name
LIMIT $limit
`

	res, err := neo4j.ExecuteQuery(ctx, c.driver, cypher, params, neo4j.EagerResultTransformer, c.execOptions()...)
	if err != nil {
		return "", fmt.Errorf("execute neo4j search: %w", err)
	}

	results := make([]searchResult, 0, len(res.Records))
	for _, record := range res.Records {
		m := record.AsMap()
		results = append(results, searchResult{
			Name:          getString(m, "name"),
			QualifiedName: getString(m, "qualified_name"),
			Kind:          getString(m, "kind"),
			Namespace:     getString(m, "namespace"),
			File:          getString(m, "file"),
			Doc:           truncateString(getString(m, "doc"), 500),
		})
	}

	payload := map[string]any{
		"query":   query,
		"results": results,
	}

	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode search results: %w", err)
	}
	return string(jsonBytes), nil
}

func (c *codeGraphTools) handleSymbolDetails(ctx context.Context, raw json.RawMessage) (string, error) {
	var args detailArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	qname := strings.TrimSpace(args.QualifiedName)
	if qname == "" {
		return "", errors.New("qualified_name must not be empty")
	}

	limit := args.RelationshipLimit
	if limit <= 0 {
		limit = defaultRelationshipLimit
	}
	if limit > maxRelationshipLimit {
		limit = maxRelationshipLimit
	}

	params := map[string]any{
		"qualified_name": qname,
	}
	cypher := `
MATCH (n:Node {qualified_name: $qualified_name})
RETURN n.name AS name,
       n.qualified_name AS qualified_name,
       n.kind AS kind,
       n.namespace AS namespace,
       n.file AS file,
       n.doc AS doc,
       n.code AS code,
       n.type AS type,
       n.underlying_type AS underlying_type,
       n.parent_qualified_name AS parent_qualified_name
`

	res, err := neo4j.ExecuteQuery(ctx, c.driver, cypher, params, neo4j.EagerResultTransformer, c.execOptions()...)
	if err != nil {
		return "", fmt.Errorf("execute node lookup: %w", err)
	}

	if len(res.Records) == 0 {
		return "", fmt.Errorf("no symbol found for %s", qname)
	}

	info := res.Records[0].AsMap()
	details := symbolDetails{
		Name:                getString(info, "name"),
		QualifiedName:       getString(info, "qualified_name"),
		Kind:                getString(info, "kind"),
		Namespace:           getString(info, "namespace"),
		File:                getString(info, "file"),
		Type:                getString(info, "type"),
		UnderlyingType:      getString(info, "underlying_type"),
		ParentQualifiedName: getString(info, "parent_qualified_name"),
		Doc:                 truncateString(getString(info, "doc"), 2000),
		Code:                truncateString(getString(info, "code"), maxCodeSnippetCharacters),
	}

	if args.IncludeRelationships || !jsonFieldProvided(raw, "include_relationships") {
		rels := &symbolRelationships{}
		if calls, err := c.fetchRelationship(ctx, qname, limit, `
MATCH (:Node {qualified_name: $qname})-[:CALLS]->(dst:Node)
RETURN dst.qualified_name AS qualified_name,
       dst.name AS name,
       dst.kind AS kind,
       dst.namespace AS namespace
ORDER BY dst.name
LIMIT $limit
`); err == nil {
			rels.Calls = calls
		} else {
			return "", err
		}
		if callers, err := c.fetchRelationship(ctx, qname, limit, `
MATCH (src:Node)-[:CALLS]->(:Node {qualified_name: $qname})
RETURN src.qualified_name AS qualified_name,
       src.name AS name,
       src.kind AS kind,
       src.namespace AS namespace
ORDER BY src.name
LIMIT $limit
`); err == nil {
			rels.CalledBy = callers
		} else {
			return "", err
		}
		if impls, err := c.fetchRelationship(ctx, qname, limit, `
MATCH (:Node {qualified_name: $qname})-[:IMPLEMENTS]->(iface:Node)
RETURN iface.qualified_name AS qualified_name,
       iface.name AS name,
       iface.kind AS kind,
       iface.namespace AS namespace
ORDER BY iface.name
LIMIT $limit
`); err == nil {
			rels.Implements = impls
		} else {
			return "", err
		}
		if implementedBy, err := c.fetchRelationship(ctx, qname, limit, `
MATCH (impl:Node)-[:IMPLEMENTS]->(:Node {qualified_name: $qname})
RETURN impl.qualified_name AS qualified_name,
       impl.name AS name,
       impl.kind AS kind,
       impl.namespace AS namespace
ORDER BY impl.name
LIMIT $limit
`); err == nil {
			rels.ImplementedBy = implementedBy
		} else {
			return "", err
		}
		if returns, err := c.fetchRelationship(ctx, qname, limit, `
MATCH (:Node {qualified_name: $qname})-[:RETURNS]->(ret:Node)
RETURN ret.qualified_name AS qualified_name,
       ret.name AS name,
       ret.kind AS kind,
       ret.namespace AS namespace
ORDER BY ret.name
LIMIT $limit
`); err == nil {
			rels.Returns = returns
		} else {
			return "", err
		}
		if params, err := c.fetchRelationship(ctx, qname, limit, `
MATCH (param:Node)-[:PARAM_OF]->(:Node {qualified_name: $qname})
RETURN param.qualified_name AS qualified_name,
       param.name AS name,
       param.kind AS kind,
       param.namespace AS namespace
ORDER BY param.name
LIMIT $limit
`); err == nil {
			rels.Params = params
		} else {
			return "", err
		}
		details.Relationships = rels
	}

	jsonBytes, err := json.MarshalIndent(details, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode detail response: %w", err)
	}
	return string(jsonBytes), nil
}

func (c *codeGraphTools) fetchRelationship(ctx context.Context, qualifiedName string, limit int, query string) ([]relatedSymbol, error) {
	params := map[string]any{
		"qname": qualifiedName,
		"limit": limit,
	}
	res, err := neo4j.ExecuteQuery(ctx, c.driver, query, params, neo4j.EagerResultTransformer, c.execOptions()...)
	if err != nil {
		return nil, fmt.Errorf("execute relationship query: %w", err)
	}
	relations := make([]relatedSymbol, 0, len(res.Records))
	for _, record := range res.Records {
		m := record.AsMap()
		relations = append(relations, relatedSymbol{
			QualifiedName: getString(m, "qualified_name"),
			Name:          getString(m, "name"),
			Kind:          getString(m, "kind"),
			Namespace:     getString(m, "namespace"),
		})
	}
	return relations, nil
}

func (c *codeGraphTools) grepDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "grep_code_nodes",
		Description: "Search within stored node code/doc/name fields for a text snippet and return contextual matches.",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Substring to search for inside node fields.",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "Optional exact namespace (Go package path) filter.",
				},
				"kinds": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional list of node kinds to include (e.g., function, struct, interface).",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     maxSearchLimit,
					"description": "Maximum number of matches to return (default 20).",
				},
				"case_sensitive": map[string]any{
					"type":        "boolean",
					"description": "When true, the match is case sensitive (default false).",
				},
				"fields": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional list of fields to search: code, doc, name, qualified_name (defaults to code+doc).",
				},
			},
			"required": []string{"query", "namespace", "kinds", "limit", "case_sensitive", "fields"},
		},
	}
}

func (c *codeGraphTools) handleGrepNodes(ctx context.Context, raw json.RawMessage) (string, error) {
	var args grepArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", errors.New("query must not be empty")
	}
	limit := args.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	fields := normalizeFields(args.Fields)
	caseSensitiveClause, insensitiveClause := buildFieldClauses(fields)
	if caseSensitiveClause == "" || insensitiveClause == "" {
		return "", errors.New("no valid fields provided for grep search")
	}

	kinds := normalizeKinds(args.Kinds)

	cypher := fmt.Sprintf(`
WITH $query AS q, toLower($query) AS lq
MATCH (n:Node)
WHERE ($namespace = "" OR n.namespace = $namespace)
  AND (size($kinds) = 0 OR toLower(n.kind) IN $kinds)
  AND (
    ($case_sensitive = true AND (%s)) OR
    ($case_sensitive = false AND (%s))
  )
RETURN n.name AS name,
       n.qualified_name AS qualified_name,
       n.kind AS kind,
       n.namespace AS namespace,
       n.file AS file,
       n.code AS code,
       n.doc AS doc
ORDER BY n.kind, n.name
LIMIT $limit
`, caseSensitiveClause, insensitiveClause)

	params := map[string]any{
		"query":          query,
		"namespace":      strings.TrimSpace(args.Namespace),
		"kinds":          kinds,
		"limit":          limit,
		"case_sensitive": args.CaseSensitive,
	}

	res, err := neo4j.ExecuteQuery(ctx, c.driver, cypher, params, neo4j.EagerResultTransformer, c.execOptions()...)
	if err != nil {
		return "", fmt.Errorf("execute neo4j grep: %w", err)
	}

	results := make([]grepResult, 0, len(res.Records))
	for _, record := range res.Records {
		m := record.AsMap()
		name := getString(m, "name")
		qualified := getString(m, "qualified_name")
		kind := getString(m, "kind")
		namespace := getString(m, "namespace")
		file := getString(m, "file")
		code := getString(m, "code")
		doc := getString(m, "doc")

		field, snippet, ok := selectMatchSnippet(fields, name, qualified, code, doc, query, args.CaseSensitive)
		if !ok {
			continue
		}

		results = append(results, grepResult{
			QualifiedName: qualified,
			Name:          name,
			Kind:          kind,
			Namespace:     namespace,
			File:          file,
			Field:         field,
			Snippet:       snippet,
		})
	}

	payload := map[string]any{
		"query":          query,
		"case_sensitive": args.CaseSensitive,
		"fields":         fields,
		"results":        results,
	}

	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode grep results: %w", err)
	}
	return string(jsonBytes), nil
}

func normalizeKinds(kinds []string) []string {
	if len(kinds) == 0 {
		return []string{}
	}
	normalized := make([]string, 0, len(kinds))
	for _, k := range kinds {
		trimmed := strings.TrimSpace(strings.ToLower(k))
		if trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	return normalized
}

func normalizeFields(fields []string) []string {
	valid := map[string]bool{
		"code":           false,
		"doc":            false,
		"name":           false,
		"qualified_name": false,
	}
	if len(fields) == 0 {
		valid["code"] = true
		valid["doc"] = true
	} else {
		for _, f := range fields {
			key := strings.TrimSpace(strings.ToLower(f))
			if _, ok := valid[key]; ok {
				valid[key] = true
			}
		}
		if !valid["code"] && !valid["doc"] && !valid["name"] && !valid["qualified_name"] {
			valid["code"] = true
			valid["doc"] = true
		}
	}
	ordered := make([]string, 0, len(valid))
	for _, key := range []string{"code", "doc", "name", "qualified_name"} {
		if valid[key] {
			ordered = append(ordered, key)
		}
	}
	return ordered
}

func buildFieldClauses(fields []string) (string, string) {
	access := map[string]string{
		"code":           "n.code",
		"doc":            "n.doc",
		"name":           "n.name",
		"qualified_name": "n.qualified_name",
	}
	caseSensitiveParts := make([]string, 0, len(fields))
	insensitiveParts := make([]string, 0, len(fields))
	for _, field := range fields {
		expr := access[field]
		caseSensitiveParts = append(caseSensitiveParts, fmt.Sprintf("%s CONTAINS q", expr))
		insensitiveParts = append(insensitiveParts, fmt.Sprintf("toLower(coalesce(%s, \"\")) CONTAINS lq", expr))
	}
	return strings.Join(caseSensitiveParts, " OR "), strings.Join(insensitiveParts, " OR ")
}

func selectMatchSnippet(fields []string, name, qualified, code, doc, query string, caseSensitive bool) (string, string, bool) {
	for _, field := range fields {
		var content string
		switch field {
		case "code":
			content = code
		case "doc":
			content = doc
		case "name":
			content = name
		case "qualified_name":
			content = qualified
		}
		snippet, ok := findSnippet(content, query, caseSensitive)
		if ok {
			return field, snippet, true
		}
	}
	return "", "", false
}

func findSnippet(content, query string, caseSensitive bool) (string, bool) {
	if content == "" {
		return "", false
	}
	var (
		index    int
		needle   string
		haystack string
	)
	haystack = content
	needle = query
	matchLen := len(needle)
	if caseSensitive {
		index = strings.Index(haystack, needle)
	} else {
		var length int
		index, length = caseInsensitiveMatch(haystack, needle)
		matchLen = length
	}
	if index < 0 {
		return "", false
	}
	start := index
	end := index + matchLen
	runes := []rune(haystack)
	startRune := utf8.RuneCountInString(haystack[:start])
	lengthRunes := utf8.RuneCountInString(haystack[start:end])
	if lengthRunes == 0 {
		lengthRunes = 1
	}
	before := startRune - grepContextRunes
	if before < 0 {
		before = 0
	}
	after := startRune + lengthRunes + grepContextRunes
	if after > len(runes) {
		after = len(runes)
	}
	snippetRunes := runes[before:after]
	if len(snippetRunes) > maxGrepSnippetRunes {
		after = before + maxGrepSnippetRunes
		if after > len(runes) {
			after = len(runes)
		}
		snippetRunes = runes[before:after]
	}
	snippet := string(snippetRunes)
	if before > 0 {
		snippet = "..." + snippet
	}
	if after < len(runes) {
		snippet = snippet + "..."
	}
	snippet = strings.ReplaceAll(snippet, "\t", "    ")
	snippet = strings.ReplaceAll(snippet, "\r", "")
	return snippet, true
}

func caseInsensitiveMatch(haystack, needle string) (int, int) {
	if needle == "" {
		return 0, 0
	}
	hRunes := []rune(haystack)
	nRunes := []rune(needle)
	needleLen := len(nRunes)
	if needleLen == 0 {
		return 0, 0
	}
	if needleLen > len(hRunes) {
		return -1, 0
	}
	prefixBytes := make([]int, len(hRunes)+1)
	for i, r := range hRunes {
		prefixBytes[i+1] = prefixBytes[i] + utf8.RuneLen(r)
	}
	for i := 0; i <= len(hRunes)-needleLen; i++ {
		segment := string(hRunes[i : i+needleLen])
		if strings.EqualFold(segment, needle) {
			start := prefixBytes[i]
			length := prefixBytes[i+needleLen] - prefixBytes[i]
			return start, length
		}
	}
	return -1, 0
}

func (c *codeGraphTools) execOptions() []neo4j.ExecuteQueryConfigurationOption {
	if strings.TrimSpace(c.database) == "" {
		return nil
	}
	return []neo4j.ExecuteQueryConfigurationOption{neo4j.ExecuteQueryWithDatabase(c.database)}
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case string:
			return val
		case fmt.Stringer:
			return val.String()
		case nil:
			return ""
		}
	}
	return ""
}

func truncateString(s string, max int) string {
	trimmed := strings.TrimSpace(s)
	if max <= 0 {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= max {
		return trimmed
	}
	return strings.TrimSpace(string(runes[:max])) + "..."
}

func jsonFieldProvided(raw json.RawMessage, field string) bool {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	_, ok := payload[field]
	return ok
}
