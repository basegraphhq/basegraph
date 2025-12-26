package process

import "fmt"

// Neo4jConfig captures the connection parameters required to open a driver.
type Neo4jConfig struct {
	URI      string
	Username string
	Password string
	Database string
}

// Validate returns an error when mandatory configuration values are missing.
func (c Neo4jConfig) Validate() error {
	if c.URI == "" {
		return fmt.Errorf("neo4j uri must be provided")
	}
	if c.Username == "" {
		return fmt.Errorf("neo4j username must be provided")
	}
	if c.Password == "" {
		return fmt.Errorf("neo4j password must be provided")
	}
	return nil
}
