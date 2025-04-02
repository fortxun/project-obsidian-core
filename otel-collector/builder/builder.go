package main

import (
	"log"
	"os"

	"go.opentelemetry.io/collector/cmd/builder"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	builderConfig := builder.Config{
		Distribution: builder.Distribution{
			Name:    "obsidian-core-collector",
			Version: "0.1.0",
			OtelColVersion: "0.96.0", 
			OutputPath:     "../collector",
			Go: builder.GoConfig{
				OS:   []string{"linux", "darwin"},
				Arch: []string{"amd64", "arm64"},
			},
		},
		Receivers: []builder.Component{
			{Name: "otlp", GoMod: "go.opentelemetry.io/collector/receiver/otlpreceiver v0.96.0"},
		},
		Processors: []builder.Component{
			{Name: "batch", GoMod: "go.opentelemetry.io/collector/processor/batchprocessor v0.96.0"},
		},
		Exporters: []builder.Component{
			{Name: "logging", GoMod: "go.opentelemetry.io/collector/exporter/loggingexporter v0.96.0"},
		},
		Extensions: []builder.Component{
			{Name: "health_check", GoMod: "go.opentelemetry.io/collector/extension/healthcheckextension v0.96.0"},
		},
	}

	// Create builder manifest
	manifestBytes, err := builderConfig.MarshalYAML()
	if err != nil {
		logger.Fatal("Failed to marshal config", zap.Error(err))
	}

	// Write builder manifest
	err = os.WriteFile("builder-config.yaml", manifestBytes, 0600)
	if err != nil {
		logger.Fatal("Failed to write config file", zap.Error(err))
	}

	// Run the builder
	if err = builder.Run([]string{"build", "--config=builder-config.yaml"}); err != nil {
		log.Fatalf("Failed to build collector: %v", err)
	}
}