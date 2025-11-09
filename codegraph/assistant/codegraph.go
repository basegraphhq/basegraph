package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
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

// RegisterCodeGraphTools exposes the Neo4j-backed code graph tools to external registries.
func RegisterCodeGraphTools(ctx context.Context, reg *ToolRegistry, cfg process.Neo4jConfig) error {
	_, err := newCodeGraphTools(ctx, reg, cfg)
	return err
}

func (c *codeGraphTools) searchDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "search_symbols",
		Description: "Search for symbols by qualified name, namespace, and kind.",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search string to match against name and qualified name.",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "Optional exact namespace filter.",
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

	includeRelationships := args.IncludeRelationships || !jsonFieldProvided(raw, "include_relationships")

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

	if includeRelationships {
		params["limit"] = limit
		cypher = `
MATCH (n:Node {qualified_name: $qualified_name})
CALL {
  WITH n
  MATCH (n)-[:CALLS]->(dst:Node)
  RETURN collect({qualified_name: dst.qualified_name,
                  name: dst.name,
                  kind: dst.kind,
                  namespace: dst.namespace})[0..$limit] AS calls
}
WITH n, calls
CALL {
  WITH n
  MATCH (src:Node)-[:CALLS]->(n)
  RETURN collect({qualified_name: src.qualified_name,
                  name: src.name,
                  kind: src.kind,
                  namespace: src.namespace})[0..$limit] AS called_by
}
WITH n, calls, called_by
CALL {
  WITH n
  MATCH (n)-[:IMPLEMENTS]->(iface:Node)
  RETURN collect({qualified_name: iface.qualified_name,
                  name: iface.name,
                  kind: iface.kind,
                  namespace: iface.namespace})[0..$limit] AS implements
}
WITH n, calls, called_by, implements
CALL {
  WITH n
  MATCH (impl:Node)-[:IMPLEMENTS]->(n)
  RETURN collect({qualified_name: impl.qualified_name,
                  name: impl.name,
                  kind: impl.kind,
                  namespace: impl.namespace})[0..$limit] AS implemented_by
}
WITH n, calls, called_by, implements, implemented_by
CALL {
  WITH n
  MATCH (n)-[:RETURNS]->(ret:Node)
  RETURN collect({qualified_name: ret.qualified_name,
                  name: ret.name,
                  kind: ret.kind,
                  namespace: ret.namespace})[0..$limit] AS returns_rel
}
WITH n, calls, called_by, implements, implemented_by, returns_rel
CALL {
  WITH n
  MATCH (param:Node)-[:PARAM_OF]->(n)
  RETURN collect({qualified_name: param.qualified_name,
                  name: param.name,
                  kind: param.kind,
                  namespace: param.namespace})[0..$limit] AS params_rel
}
RETURN n.name AS name,
       n.qualified_name AS qualified_name,
       n.kind AS kind,
       n.namespace AS namespace,
       n.file AS file,
       n.doc AS doc,
       n.code AS code,
       n.type AS type,
       n.underlying_type AS underlying_type,
       n.parent_qualified_name AS parent_qualified_name,
       calls,
       called_by,
       implements,
       implemented_by,
       returns_rel,
       params_rel
`
	}

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

	if includeRelationships {
		rels := &symbolRelationships{
			Calls:         toRelatedSymbols(info["calls"]),
			CalledBy:      toRelatedSymbols(info["called_by"]),
			Implements:    toRelatedSymbols(info["implements"]),
			ImplementedBy: toRelatedSymbols(info["implemented_by"]),
			Returns:       toRelatedSymbols(info["returns_rel"]),
			Params:        toRelatedSymbols(info["params_rel"]),
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
	whereClause := buildRegexClause(fields)
	if whereClause == "" {
		return "", errors.New("no valid fields provided for grep search")
	}

	kinds := normalizeKinds(args.Kinds)

	dbPattern, snippetPattern, err := buildRegexPatterns(query, args.CaseSensitive)
	if err != nil {
		return "", err
	}
	var snippetRE *regexp.Regexp
	if snippetPattern != "" {
		if re, compileErr := regexp.Compile(snippetPattern); compileErr == nil {
			snippetRE = re
		}
	}

	cypher := fmt.Sprintf(`
MATCH (n:Node)
WHERE ($namespace = "" OR n.namespace = $namespace)
  AND (size($kinds) = 0 OR toLower(n.kind) IN $kinds)
  AND (%s)
RETURN n.name AS name,
       n.qualified_name AS qualified_name,
       n.kind AS kind,
       n.namespace AS namespace,
       n.file AS file,
       n.code AS code,
       n.doc AS doc
ORDER BY n.kind, n.name
LIMIT $limit
`, whereClause)

	params := map[string]any{
		"query":     query,
		"namespace": strings.TrimSpace(args.Namespace),
		"kinds":     kinds,
		"limit":     limit,
		"pattern":   dbPattern,
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

		field, snippet, ok := selectMatchSnippet(fields, name, qualified, code, doc, snippetRE, query, args.CaseSensitive)
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
		"pattern":        dbPattern,
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

func buildRegexClause(fields []string) string {
	access := map[string]string{
		"code":           "n.code",
		"doc":            "n.doc",
		"name":           "n.name",
		"qualified_name": "n.qualified_name",
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		expr := access[field]
		parts = append(parts, fmt.Sprintf("coalesce(%s, \"\") =~ $pattern", expr))
	}
	return strings.Join(parts, " OR ")
}

func buildRegexPatterns(query string, caseSensitive bool) (string, string, error) {
	flagsFromQuery, rawBody := splitInlineFlags(strings.TrimSpace(query))
	if rawBody == "" {
		return "", "", errors.New("query must not be empty")
	}
	flagSet := map[rune]struct{}{}
	for _, r := range flagsFromQuery {
		flagSet[r] = struct{}{}
	}
	flagSet['s'] = struct{}{}
	if !caseSensitive {
		flagSet['i'] = struct{}{}
	}

	flags := make([]rune, 0, len(flagSet))
	for r := range flagSet {
		flags = append(flags, r)
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i] < flags[j] })

	prefix := ""
	if len(flags) > 0 {
		prefix = "(?" + string(flags) + ")"
	}

	build := func(patternBody string) (string, string) {
		hasPrefixAnchor := strings.HasPrefix(patternBody, "^")
		hasSuffixAnchor := strings.HasSuffix(patternBody, "$")

		dbBody := patternBody
		if !hasPrefixAnchor {
			dbBody = ".*" + dbBody
		}
		if !hasSuffixAnchor {
			dbBody = dbBody + ".*"
		}
		return prefix + dbBody, prefix + patternBody
	}

	dbPattern, snippetPattern := build(rawBody)
	if _, err := regexp.Compile(snippetPattern); err != nil {
		escapedBody := regexp.QuoteMeta(rawBody)
		if escapedBody == rawBody {
			return "", "", fmt.Errorf("compile regex: %w", err)
		}
		dbPattern, snippetPattern = build(escapedBody)
		if _, escapedErr := regexp.Compile(snippetPattern); escapedErr != nil {
			return "", "", fmt.Errorf("compile regex after escaping: %w", escapedErr)
		}
	}
	return dbPattern, snippetPattern, nil
}

func splitInlineFlags(pattern string) (string, string) {
	if strings.HasPrefix(pattern, "(?") {
		if idx := strings.Index(pattern, ")"); idx > 2 {
			flags := pattern[2:idx]
			if !strings.ContainsAny(flags, ":<") {
				return flags, pattern[idx+1:]
			}
		}
	}
	return "", pattern
}

func selectMatchSnippet(fields []string, name, qualified, code, doc string, re *regexp.Regexp, query string, caseSensitive bool) (string, string, bool) {
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
		snippet, ok := findSnippet(content, re, query, caseSensitive)
		if ok {
			return field, snippet, true
		}
	}
	return "", "", false
}

func findSnippet(content string, re *regexp.Regexp, query string, caseSensitive bool) (string, bool) {
	if content == "" {
		return "", false
	}
	if re != nil {
		if loc := re.FindStringIndex(content); loc != nil {
			return buildSnippet(content, loc[0], loc[1]), true
		}
	}
	if caseSensitive {
		if idx := strings.Index(content, query); idx >= 0 {
			return buildSnippet(content, idx, idx+len(query)), true
		}
	} else {
		if idx, length := caseInsensitiveMatch(content, query); idx >= 0 {
			return buildSnippet(content, idx, idx+length), true
		}
	}
	return "", false
}

func buildSnippet(content string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(content) {
		end = len(content)
	}
	if start >= end {
		return ""
	}
	runes := []rune(content)
	startRune := utf8.RuneCountInString(content[:start])
	lengthRunes := utf8.RuneCountInString(content[start:end])
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
	return snippet
}

func toRelatedSymbols(value any) []relatedSymbol {
	list, ok := value.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	related := make([]relatedSymbol, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			related = append(related, relatedSymbol{
				QualifiedName: getString(m, "qualified_name"),
				Name:          getString(m, "name"),
				Kind:          getString(m, "kind"),
				Namespace:     getString(m, "namespace"),
			})
		}
	}
	return related
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
