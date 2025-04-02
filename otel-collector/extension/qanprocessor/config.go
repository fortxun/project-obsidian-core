package qanprocessor

import (
	"time"

	"go.opentelemetry.io/collector/component"
)

// Config defines configuration for QAN processor.
type Config struct {
	// MySQL configuration
	MySQL MySQLConfig `mapstructure:"mysql"`

	// PostgreSQL configuration
	PostgreSQL PostgreSQLConfig `mapstructure:"postgresql"`
}

// MySQLConfig defines MySQL-specific QAN configuration.
type MySQLConfig struct {
	// Enabled indicates whether MySQL QAN collection is enabled
	Enabled bool `mapstructure:"enabled"`

	// Endpoint in format host:port
	Endpoint string `mapstructure:"endpoint"`

	// Username for MySQL connection
	Username string `mapstructure:"username"`

	// Password for MySQL connection
	Password string `mapstructure:"password"`

	// Database to connect to
	Database string `mapstructure:"database"`

	// CollectionInterval in seconds
	// Can be set to "adaptive" to use adaptive polling
	CollectionInterval string `mapstructure:"collection_interval"`

	// AdaptiveConfig contains configuration for adaptive polling
	AdaptiveConfig AdaptiveConfig `mapstructure:"adaptive"`
}

// PostgreSQLConfig defines PostgreSQL-specific QAN configuration.
type PostgreSQLConfig struct {
	// Enabled indicates whether PostgreSQL QAN collection is enabled
	Enabled bool `mapstructure:"enabled"`

	// Endpoint in format host:port
	Endpoint string `mapstructure:"endpoint"`

	// Username for PostgreSQL connection
	Username string `mapstructure:"username"`

	// Password for PostgreSQL connection
	Password string `mapstructure:"password"`

	// Database to connect to
	Database string `mapstructure:"database"`

	// CollectionInterval in seconds
	CollectionInterval int `mapstructure:"collection_interval"`
}

// AdaptiveConfig defines settings for adaptive polling intervals.
type AdaptiveConfig struct {
	// BaseInterval is the starting point for adaptive interval calculations (in seconds)
	BaseInterval int `mapstructure:"base_interval"`

	// StateDirectory is where governor state is persisted
	StateDirectory string `mapstructure:"state_directory"`

	// Enabled explicitly enables or disables adaptive polling
	Enabled bool `mapstructure:"enabled"`
}

var _ component.Config = (*Config)(nil)

// Validate checks if the processor configuration is valid
func (cfg *Config) Validate() error {
	return nil
}
