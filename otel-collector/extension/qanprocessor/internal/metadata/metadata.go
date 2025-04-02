// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package metadata

import (
	"go.opentelemetry.io/collector/component"
)

// Constants for the QAN processor.
const (
	Type = "qanprocessor"
)

var (
	// Version is the version of the QAN processor.
	// This will be set by the build system.
	Version = "0.1.0"
)

// BuildInfo provides the component build information.
func BuildInfo() component.BuildInfo {
	return component.BuildInfo{
		Command:     Type,
		Description: "Query Analytics (QAN) processor for MySQL and PostgreSQL",
		Version:     Version,
	}
}