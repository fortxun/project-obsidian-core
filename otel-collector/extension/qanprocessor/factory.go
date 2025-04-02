// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package qanprocessor

import (
	"context"

	"github.com/project-obsidian-core/otel-collector/extension/qanprocessor/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
)

const (
	// The value of "type" key in configuration.
	typeStr = "qanprocessor"
)

// NewFactory returns a new factory for the QAN processor.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeStr,
		createDefaultConfig,
		processor.WithLogs(createLogsProcessor, component.StabilityLevelDevelopment),
	)
}

// createDefaultConfig creates the default configuration for the processor.
func createDefaultConfig() component.Config {
	return &Config{
		MySQL: MySQLConfig{
			Enabled:            true,
			CollectionInterval: 60,
		},
		PostgreSQL: PostgreSQLConfig{
			Enabled:            true,
			CollectionInterval: 60,
		},
	}
}

// createLogsProcessor creates a logs processor based on the config.
func createLogsProcessor(
	ctx context.Context,
	set processor.CreateSettings,
	cfg component.Config,
	nextConsumer consumer.Logs,
) (processor.Logs, error) {
	pCfg := cfg.(*Config)
	return newQANProcessor(set.Logger, pCfg, nextConsumer)
}