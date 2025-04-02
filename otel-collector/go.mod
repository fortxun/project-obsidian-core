module github.com/project-obsidian-core/otel-collector

go 1.21

require (
	github.com/open-telemetry/opentelemetry-collector v0.96.0
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/otlpexporter v0.96.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mysqlreceiver v0.96.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/postgresqlreceiver v0.96.0
	go.opentelemetry.io/collector/exporter v0.96.0
	go.opentelemetry.io/collector/processor v0.96.0
	go.opentelemetry.io/collector/receiver v0.96.0
	go.uber.org/zap v1.27.0
)