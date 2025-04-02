module github.com/ewen/project-obsidian-core/otel-collector/extension/qanprocessor

go 1.21

require (
	github.com/go-sql-driver/mysql v1.7.1
	github.com/lib/pq v1.10.9
	go.opentelemetry.io/collector/component v0.96.0
	go.opentelemetry.io/collector/consumer v0.96.0
	go.opentelemetry.io/collector/pdata v1.4.0
	go.uber.org/zap v1.27.0
)