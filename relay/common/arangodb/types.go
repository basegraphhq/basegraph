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
