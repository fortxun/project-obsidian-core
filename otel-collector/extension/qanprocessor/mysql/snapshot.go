// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"sync"
	"time"
)

// DigestData represents a single row from MySQL performance_schema.events_statements_summary_by_digest
type DigestData struct {
	// Digest is the normalized statement digest hash from MySQL
	Digest string

	// SchemaName is the schema/database name
	SchemaName string

	// DigestText is a sample of the SQL statement
	DigestText string

	// CountStar is the number of times this statement was executed
	CountStar int64

	// SumTimerWait is the total execution time (in picoseconds)
	SumTimerWait int64

	// SumLockTime is the total time spent waiting for locks (in picoseconds)
	SumLockTime int64

	// SumErrors is the total number of errors
	SumErrors int64

	// SumWarnings is the total number of warnings
	SumWarnings int64

	// SumRowsAffected is the total number of rows affected
	SumRowsAffected int64

	// SumRowsSent is the total number of rows sent to the client
	SumRowsSent int64

	// SumRowsExamined is the total number of rows examined
	SumRowsExamined int64

	// SumCreatedTmpTables is the total number of temp tables created
	SumCreatedTmpTables int64

	// SumCreatedTmpDiskTables is the total number of on-disk temp tables created
	SumCreatedTmpDiskTables int64

	// SumSortRows is the total number of sorted rows
	SumSortRows int64

	// SumNoIndexUsed is the number of times no index was used
	SumNoIndexUsed int64

	// SumNoGoodIndexUsed is the number of times a suboptimal index was used
	SumNoGoodIndexUsed int64

	// Timestamp when this data was collected
	Timestamp time.Time
}

// Snapshot represents a point-in-time collection of all statement digests
type Snapshot struct {
	// Digests maps digest hash to its data
	Digests map[string]DigestData

	// Timestamp when this snapshot was taken
	Timestamp time.Time

	// InstanceID is the identifier for the database instance
	InstanceID string
}

// SnapshotStore stores and manages snapshots for calculating deltas
type SnapshotStore struct {
	mu              sync.RWMutex
	latestSnapshots map[string]*Snapshot // Keyed by instanceID
}

// NewSnapshotStore creates a new snapshot store
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{
		latestSnapshots: make(map[string]*Snapshot),
	}
}

// StoreSnapshot stores a new snapshot for an instance
func (s *SnapshotStore) StoreSnapshot(instanceID string, snapshot *Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latestSnapshots[instanceID] = snapshot
}

// GetSnapshot retrieves the latest snapshot for an instance
func (s *SnapshotStore) GetSnapshot(instanceID string) *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latestSnapshots[instanceID]
}

// DeltaResult holds the calculated delta between two snapshots for a single digest
type DeltaResult struct {
	// Digest is the normalized statement digest hash
	Digest string

	// SchemaName is the schema/database name
	SchemaName string

	// DigestText is a sample of the SQL statement
	DigestText string

	// TimePeriodSecs is the time period in seconds that this delta covers
	TimePeriodSecs float64

	// DeltaCountStar is the change in execution count
	DeltaCountStar int64

	// DeltaSumTimerWait is the change in total execution time
	DeltaSumTimerWait int64

	// DeltaSumLockTime is the change in lock time
	DeltaSumLockTime int64

	// DeltaSumErrors is the change in error count
	DeltaSumErrors int64

	// DeltaSumWarnings is the change in warning count
	DeltaSumWarnings int64

	// DeltaSumRowsAffected is the change in rows affected
	DeltaSumRowsAffected int64

	// DeltaSumRowsSent is the change in rows sent
	DeltaSumRowsSent int64

	// DeltaSumRowsExamined is the change in rows examined
	DeltaSumRowsExamined int64

	// DeltaSumCreatedTmpTables is the change in temp tables created
	DeltaSumCreatedTmpTables int64

	// DeltaSumCreatedTmpDiskTables is the change in on-disk temp tables created
	DeltaSumCreatedTmpDiskTables int64

	// DeltaSumSortRows is the change in sorted rows
	DeltaSumSortRows int64

	// DeltaSumNoIndexUsed is the change in no index used count
	DeltaSumNoIndexUsed int64

	// DeltaSumNoGoodIndexUsed is the change in suboptimal index used count
	DeltaSumNoGoodIndexUsed int64
}

// CalculateDeltas computes the deltas between the previous and current snapshots
func CalculateDeltas(prev, curr *Snapshot) []DeltaResult {
	if prev == nil || curr == nil {
		return nil
	}

	results := make([]DeltaResult, 0)
	timeDiffSecs := curr.Timestamp.Sub(prev.Timestamp).Seconds()

	// Process all digests in the current snapshot
	for digest, currData := range curr.Digests {
		// Get previous data for this digest, if it exists
		prevData, exists := prev.Digests[digest]

		// If this is a new digest that didn't exist in the previous snapshot,
		// we consider all its values as the delta
		if !exists {
			results = append(results, DeltaResult{
				Digest:                     digest,
				SchemaName:                 currData.SchemaName,
				DigestText:                 currData.DigestText,
				TimePeriodSecs:             timeDiffSecs,
				DeltaCountStar:             currData.CountStar,
				DeltaSumTimerWait:          currData.SumTimerWait,
				DeltaSumLockTime:           currData.SumLockTime,
				DeltaSumErrors:             currData.SumErrors,
				DeltaSumWarnings:           currData.SumWarnings,
				DeltaSumRowsAffected:       currData.SumRowsAffected,
				DeltaSumRowsSent:           currData.SumRowsSent,
				DeltaSumRowsExamined:       currData.SumRowsExamined,
				DeltaSumCreatedTmpTables:   currData.SumCreatedTmpTables,
				DeltaSumCreatedTmpDiskTables: currData.SumCreatedTmpDiskTables,
				DeltaSumSortRows:           currData.SumSortRows,
				DeltaSumNoIndexUsed:        currData.SumNoIndexUsed,
				DeltaSumNoGoodIndexUsed:    currData.SumNoGoodIndexUsed,
			})
			continue
		}

		// Calculate deltas for existing digests
		// Handle potential counter resets (when current value is less than previous)
		var deltaCountStar int64
		if currData.CountStar >= prevData.CountStar {
			deltaCountStar = currData.CountStar - prevData.CountStar
		} else {
			deltaCountStar = currData.CountStar // Counter reset case
		}

		// Only include digests that have been executed during this interval
		if deltaCountStar > 0 {
			// Helper function to handle counter resets
			calcDelta := func(curr, prev int64) int64 {
				if curr >= prev {
					return curr - prev
				}
				return curr // Assume a reset occurred
			}

			results = append(results, DeltaResult{
				Digest:                     digest,
				SchemaName:                 currData.SchemaName,
				DigestText:                 currData.DigestText,
				TimePeriodSecs:             timeDiffSecs,
				DeltaCountStar:             deltaCountStar,
				DeltaSumTimerWait:          calcDelta(currData.SumTimerWait, prevData.SumTimerWait),
				DeltaSumLockTime:           calcDelta(currData.SumLockTime, prevData.SumLockTime),
				DeltaSumErrors:             calcDelta(currData.SumErrors, prevData.SumErrors),
				DeltaSumWarnings:           calcDelta(currData.SumWarnings, prevData.SumWarnings),
				DeltaSumRowsAffected:       calcDelta(currData.SumRowsAffected, prevData.SumRowsAffected),
				DeltaSumRowsSent:           calcDelta(currData.SumRowsSent, prevData.SumRowsSent),
				DeltaSumRowsExamined:       calcDelta(currData.SumRowsExamined, prevData.SumRowsExamined),
				DeltaSumCreatedTmpTables:   calcDelta(currData.SumCreatedTmpTables, prevData.SumCreatedTmpTables),
				DeltaSumCreatedTmpDiskTables: calcDelta(currData.SumCreatedTmpDiskTables, prevData.SumCreatedTmpDiskTables),
				DeltaSumSortRows:           calcDelta(currData.SumSortRows, prevData.SumSortRows),
				DeltaSumNoIndexUsed:        calcDelta(currData.SumNoIndexUsed, prevData.SumNoIndexUsed),
				DeltaSumNoGoodIndexUsed:    calcDelta(currData.SumNoGoodIndexUsed, prevData.SumNoGoodIndexUsed),
			})
		}
	}

	return results
}