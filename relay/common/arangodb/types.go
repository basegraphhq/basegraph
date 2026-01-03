package arangodb

type Direction string

const (
	DirectionOutbound Direction = "outbound"
	DirectionInbound  Direction = "inbound"
	DirectionAny      Direction = "any"
)

type Node struct {
	QName     string
	Name      string
	Kind      string
	Doc       string
	Filepath  string
	Namespace string
	Language  string
	Pos       int
	End       int
	IsMethod  bool   // Go: true for receiver functions
	TypeQName string // For members: the type of the field/variable
	Signature string // For functions: human-readable signature
}

type Edge struct {
	From       string
	To         string
	FromKind   string
	ToKind     string
	Properties map[string]any
}

type GraphNode struct {
	QName    string
	Name     string
	Kind     string
	Filepath string
}

type GraphEdge struct {
	From string
	To   string
	Type string
}

type TraversalOptions struct {
	EdgeTypes []string
	Direction Direction
	MaxDepth  int
}

// FileSymbol represents a symbol found in a file (for symbols operation).
type FileSymbol struct {
	QName     string
	Name      string
	Kind      string
	Signature string
	Pos       int
	End       int
}

// SearchOptions configures symbol search parameters.
type SearchOptions struct {
	Name      string // Glob pattern: "Plan*", "*Issue*"
	Kind      string // Filter by kind: function, method, struct, interface
	File      string // Filter by filepath
	Namespace string // Filter by module path
}

// SearchResult represents a symbol found by search.
type SearchResult struct {
	QName     string
	Name      string
	Kind      string
	Signature string
	Filepath  string
	Pos       int
}
