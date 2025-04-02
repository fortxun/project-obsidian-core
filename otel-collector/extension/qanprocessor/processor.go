// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package qanprocessor

import (
	"context"
	"sync"
	"time"

	"github.com/project-obsidian-core/otel-collector/extension/qanprocessor/mysql"
	"github.com/project-obsidian-core/otel-collector/extension/qanprocessor/postgresql"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

// qanProcessor implements the processor.Logs interface for
// collecting and processing Query Analytics (QAN) data.
type qanProcessor struct {
	logger       *zap.Logger
	config       *Config
	nextConsumer consumer.Logs

	// Snapshot storage for delta calculations
	mysqlSnapshots     *mysql.SnapshotStore
	postgresqlSnapshots *postgresql.SnapshotStore

	// Collection components
	mysqlCollector      *mysql.Collector           // Standard fixed-interval collector
	mysqlAdaptiveCollector *mysql.AdaptiveCollector  // Adaptive interval collector
	postgresqlCollector *postgresql.Collector

	shutdownCh chan struct{}
	wg         sync.WaitGroup
}

// newQANProcessor creates a new logs processor for QAN data.
func newQANProcessor(
	logger *zap.Logger,
	config *Config,
	nextConsumer consumer.Logs,
) (*qanProcessor, error) {
	processor := &qanProcessor{
		logger:       logger,
		config:       config,
		nextConsumer: nextConsumer,
		shutdownCh:   make(chan struct{}),
	}

	// Initialize snapshot stores
	processor.mysqlSnapshots = mysql.NewSnapshotStore()
	processor.postgresqlSnapshots = postgresql.NewSnapshotStore()

	// Initialize collectors
	if config.MySQL.Enabled {
		// Check if we should use adaptive collection
		if config.MySQL.CollectionInterval == "adaptive" || config.MySQL.AdaptiveConfig.Enabled {
			// Use adaptive collector
			baseIntervalSec := config.MySQL.AdaptiveConfig.BaseInterval
			if baseIntervalSec <= 0 {
				baseIntervalSec = 1 // Default to 1 second base interval
			}
			
			stateDir := config.MySQL.AdaptiveConfig.StateDirectory
			if stateDir == "" {
				stateDir = "/var/otel/governor_state"
			}
			
			mysqlAdaptiveCollector, err := mysql.NewAdaptiveCollector(
				logger,
				config.MySQL.Endpoint,
				config.MySQL.Username,
				config.MySQL.Password,
				config.MySQL.Database,
				processor.mysqlSnapshots,
				time.Duration(baseIntervalSec) * time.Second,
				stateDir,
			)
			if err != nil {
				return nil, err
			}
			processor.mysqlAdaptiveCollector = mysqlAdaptiveCollector
			
			logger.Info("MySQL QAN collection will use adaptive polling",
				zap.String("endpoint", config.MySQL.Endpoint),
				zap.Int("base_interval_sec", baseIntervalSec),
				zap.String("state_dir", stateDir))
		} else {
			// Use standard fixed-interval collector
			mysqlCollector, err := mysql.NewCollector(
				logger,
				config.MySQL.Endpoint,
				config.MySQL.Username,
				config.MySQL.Password,
				config.MySQL.Database,
				processor.mysqlSnapshots,
			)
			if err != nil {
				return nil, err
			}
			processor.mysqlCollector = mysqlCollector
			
			// Parse interval (already validated)
			var intervalSec int
			fmt.Sscanf(config.MySQL.CollectionInterval, "%d", &intervalSec)
			logger.Info("MySQL QAN collection will use fixed interval",
				zap.String("endpoint", config.MySQL.Endpoint),
				zap.Int("interval_sec", intervalSec))
		}
	}

	if config.PostgreSQL.Enabled {
		postgresqlCollector, err := postgresql.NewCollector(
			logger,
			config.PostgreSQL.Endpoint,
			config.PostgreSQL.Username,
			config.PostgreSQL.Password,
			config.PostgreSQL.Database,
			processor.postgresqlSnapshots,
		)
		if err != nil {
			return nil, err
		}
		processor.postgresqlCollector = postgresqlCollector
	}

	return processor, nil
}

// Start starts the periodic collection of QAN data.
func (p *qanProcessor) Start(ctx context.Context, host component.Host) error {
	// Start MySQL collection
	if p.config.MySQL.Enabled {
		if p.mysqlAdaptiveCollector != nil {
			// Start the adaptive collector
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				
				// Define callback for handling logs
				logsCallback := func(logs plog.Logs, err error) {
					if err != nil {
						p.logger.Error("Failed to collect MySQL QAN data", zap.Error(err))
						return
					}
					
					if logs.LogRecordCount() > 0 {
						err = p.nextConsumer.ConsumeLogs(ctx, logs)
						if err != nil {
							p.logger.Error("Failed to send MySQL QAN logs", zap.Error(err))
						}
					}
				}
				
				// Start adaptive collection
				if err := p.mysqlAdaptiveCollector.StartCollection(ctx, logsCallback); err != nil {
					p.logger.Error("Failed to start adaptive MySQL collection", zap.Error(err))
					return
				}
				
				// Wait for shutdown
				select {
				case <-p.shutdownCh:
					p.mysqlAdaptiveCollector.StopCollection()
					return
				case <-ctx.Done():
					p.mysqlAdaptiveCollector.StopCollection()
					return
				}
			}()
			
			p.logger.Info("Started adaptive MySQL QAN collection")
		} else if p.mysqlCollector != nil {
			// Use standard fixed-interval collection
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				
				// Parse interval (already validated during initialization)
				var intervalSec int
				fmt.Sscanf(p.config.MySQL.CollectionInterval, "%d", &intervalSec)
				collectionInterval := time.Duration(intervalSec) * time.Second
				
				ticker := time.NewTicker(collectionInterval)
				defer ticker.Stop()

				for {
					select {
					case <-ticker.C:
						logs, err := p.mysqlCollector.Collect(ctx)
						if err != nil {
							p.logger.Error("Failed to collect MySQL QAN data", zap.Error(err))
							continue
						}
						if logs.LogRecordCount() > 0 {
							err = p.nextConsumer.ConsumeLogs(ctx, logs)
							if err != nil {
								p.logger.Error("Failed to send MySQL QAN logs", zap.Error(err))
							}
						}
					case <-p.shutdownCh:
						return
					case <-ctx.Done():
						return
					}
				}
			}()
			
			p.logger.Info("Started fixed-interval MySQL QAN collection", 
				zap.String("interval", p.config.MySQL.CollectionInterval))
		}
	}

	// Start PostgreSQL collection
	if p.config.PostgreSQL.Enabled && p.postgresqlCollector != nil {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			collectionInterval := time.Duration(p.config.PostgreSQL.CollectionInterval) * time.Second
			ticker := time.NewTicker(collectionInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					logs, err := p.postgresqlCollector.Collect(ctx)
					if err != nil {
						p.logger.Error("Failed to collect PostgreSQL QAN data", zap.Error(err))
						continue
					}
					if logs.LogRecordCount() > 0 {
						err = p.nextConsumer.ConsumeLogs(ctx, logs)
						if err != nil {
							p.logger.Error("Failed to send PostgreSQL QAN logs", zap.Error(err))
						}
					}
				case <-p.shutdownCh:
					return
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	return nil
}

// processLogs passes all logs directly to the next consumer unmodified.
// This processor generates logs from the collectors, not from incoming data.
func (p *qanProcessor) processLogs(ctx context.Context, ld plog.Logs) (plog.Logs, error) {
	return ld, nil
}

// ConsumeLogs passes logs to the next consumer (no modification).
func (p *qanProcessor) ConsumeLogs(ctx context.Context, ld plog.Logs) error {
	processedLogs, err := p.processLogs(ctx, ld)
	if err != nil {
		return err
	}
	return p.nextConsumer.ConsumeLogs(ctx, processedLogs)
}

// Shutdown stops the QAN processor collection goroutines.
func (p *qanProcessor) Shutdown(ctx context.Context) error {
	close(p.shutdownCh)
	p.wg.Wait()
	
	// Close collectors
	var errs []error
	
	if p.mysqlAdaptiveCollector != nil {
		if err := p.mysqlAdaptiveCollector.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close MySQL adaptive collector: %w", err))
		}
	}
	
	if p.mysqlCollector != nil {
		if err := p.mysqlCollector.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close MySQL collector: %w", err))
		}
	}
	
	if p.postgresqlCollector != nil {
		if err := p.postgresqlCollector.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close PostgreSQL collector: %w", err))
		}
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}
	
	return nil
}

// Capabilities returns the processor's capabilities.
func (p *qanProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}