package id

import (
	"sync"

	"github.com/bwmarrin/snowflake"
)

var (
	node *snowflake.Node
	once sync.Once
)

// Init initializes the Snowflake node with the given node ID.
func Init(nodeID int64) error {
	var err error
	once.Do(func() {
		node, err = snowflake.NewNode(nodeID)
	})
	return err
}

// New generates a new globally unique int64 ID using the Snowflake algorithm.
// IDs are time-ordered and unique across distributed instances.
func New() int64 {
	return node.Generate().Int64()
}
