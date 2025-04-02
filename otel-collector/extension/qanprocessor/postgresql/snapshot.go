// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package postgresql

import (
	"sync"
	"time"
)

// QueryData represents a single row from PostgreSQL pg_stat_statements
type QueryData struct {
	// QueryID is the internal query identifier
	QueryID string

	// UserID is the database user identifier
	UserID string

	// DBID is the database identifier
	DBID string

	// Query is the normalized SQL statement
	Query string

	// Calls is the number of times this statement was executed
	Calls int64

	// TotalPlanTime is the total time spent planning (microseconds)
	TotalPlanTime float64

	// TotalExecTime is the total time spent executing (microseconds)
	TotalExecTime float64

	// Rows is the total number of rows processed
	Rows int64

	// SharedBlksHit is the total shared blocks hit
	SharedBlksHit int64

	// SharedBlksRead is the total shared blocks read
	SharedBlksRead int64

	// SharedBlksDirtied is the total shared blocks dirtied
	SharedBlksDirtied int64

	// SharedBlksWritten is the total shared blocks written
	SharedBlksWritten int64

	// LocalBlksHit is the total local blocks hit
	LocalBlksHit int64

	// LocalBlksRead is the total local blocks read
	LocalBlksRead int64

	// LocalBlksDirtied is the total local blocks dirtied
	LocalBlksDirtied int64

	// LocalBlksWritten is the total local blocks written
	LocalBlksWritten int64

	// TempBlksRead is the total temp blocks read
	TempBlksRead int64

	// TempBlksWritten is the total temp blocks written
	TempBlksWritten int64

	// BlkReadTime is the total time spent reading blocks (milliseconds)
	BlkReadTime float64

	// BlkWriteTime is the total time spent writing blocks (milliseconds)
	BlkWriteTime float64

	// Timestamp when this data was collected
	Timestamp time.Time
}

// Snapshot represents a point-in-time collection of all query statements
type Snapshot struct {
	// Queries maps query ID to its data
	Queries map[string]QueryData

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

// DeltaResult holds the calculated delta between two snapshots for a single query
type DeltaResult struct {
	// QueryID is the internal query identifier
	QueryID string

	// UserID is the database user identifier
	UserID string

	// DBID is the database identifier
	DBID string

	// Query is the normalized SQL statement
	Query string

	// TimePeriodSecs is the time period in seconds that this delta covers
	TimePeriodSecs float64

	// DeltaCalls is the change in execution count
	DeltaCalls int64

	// DeltaTotalPlanTime is the change in plan time
	DeltaTotalPlanTime float64

	// DeltaTotalExecTime is the change in execution time
	DeltaTotalExecTime float64

	// DeltaRows is the change in rows processed
	DeltaRows int64

	// DeltaSharedBlksHit is the change in shared blocks hit
	DeltaSharedBlksHit int64

	// DeltaSharedBlksRead is the change in shared blocks read
	DeltaSharedBlksRead int64

	// DeltaSharedBlksDirtied is the change in shared blocks dirtied
	DeltaSharedBlksDirtied int64

	// DeltaSharedBlksWritten is the change in shared blocks written
	DeltaSharedBlksWritten int64

	// DeltaLocalBlksHit is the change in local blocks hit
	DeltaLocalBlksHit int64

	// DeltaLocalBlksRead is the change in local blocks read
	DeltaLocalBlksRead int64

	// DeltaLocalBlksDirtied is the change in local blocks dirtied
	DeltaLocalBlksDirtied int64

	// DeltaLocalBlksWritten is the change in local blocks written
	DeltaLocalBlksWritten int64

	// DeltaTempBlksRead is the change in temp blocks read
	DeltaTempBlksRead int64

	// DeltaTempBlksWritten is the change in temp blocks written
	DeltaTempBlksWritten int64

	// DeltaBlkReadTime is the change in block read time
	DeltaBlkReadTime float64

	// DeltaBlkWriteTime is the change in block write time
	DeltaBlkWriteTime float64
}

// CalculateDeltas computes the deltas between the previous and current snapshots
func CalculateDeltas(prev, curr *Snapshot) []DeltaResult {
	if prev == nil || curr == nil {
		return nil
	}

	results := make([]DeltaResult, 0)
	timeDiffSecs := curr.Timestamp.Sub(prev.Timestamp).Seconds()

	// Process all queries in the current snapshot
	for queryID, currData := range curr.Queries {
		// Get previous data for this query, if it exists
		prevData, exists := prev.Queries[queryID]

		// If this is a new query that didn't exist in the previous snapshot,
		// we consider all its values as the delta
		if !exists {
			results = append(results, DeltaResult{
				QueryID:                queryID,
				UserID:                 currData.UserID,
				DBID:                   currData.DBID,
				Query:                  currData.Query,
				TimePeriodSecs:         timeDiffSecs,
				DeltaCalls:             currData.Calls,
				DeltaTotalPlanTime:     currData.TotalPlanTime,
				DeltaTotalExecTime:     currData.TotalExecTime,
				DeltaRows:              currData.Rows,
				DeltaSharedBlksHit:     currData.SharedBlksHit,
				DeltaSharedBlksRead:    currData.SharedBlksRead,
				DeltaSharedBlksDirtied: currData.SharedBlksDirtied,
				DeltaSharedBlksWritten: currData.SharedBlksWritten,
				DeltaLocalBlksHit:      currData.LocalBlksHit,
				DeltaLocalBlksRead:     currData.LocalBlksRead,
				DeltaLocalBlksDirtied:  currData.LocalBlksDirtied,
				DeltaLocalBlksWritten:  currData.LocalBlksWritten,
				DeltaTempBlksRead:      currData.TempBlksRead,
				DeltaTempBlksWritten:   currData.TempBlksWritten,
				DeltaBlkReadTime:       currData.BlkReadTime,
				DeltaBlkWriteTime:      currData.BlkWriteTime,
			})
			continue
		}

		// Calculate deltas for existing queries
		// Handle potential counter resets (when current value is less than previous)
		var deltaCalls int64
		if currData.Calls >= prevData.Calls {
			deltaCalls = currData.Calls - prevData.Calls
		} else {
			deltaCalls = currData.Calls // Counter reset case
		}

		// Only include queries that have been executed during this interval
		if deltaCalls > 0 {
			// Helper function to handle counter resets for int64
			calcDeltaInt64 := func(curr, prev int64) int64 {
				if curr >= prev {
					return curr - prev
				}
				return curr // Assume a reset occurred
			}

			// Helper function to handle counter resets for float64
			calcDeltaFloat64 := func(curr, prev float64) float64 {
				if curr >= prev {
					return curr - prev
				}
				return curr // Assume a reset occurred
			}

			results = append(results, DeltaResult{
				QueryID:                 queryID,
				UserID:                  currData.UserID,
				DBID:                    currData.DBID,
				Query:                   currData.Query,
				TimePeriodSecs:          timeDiffSecs,
				DeltaCalls:              deltaCalls,
				DeltaTotalPlanTime:      calcDeltaFloat64(currData.TotalPlanTime, prevData.TotalPlanTime),
				DeltaTotalExecTime:      calcDeltaFloat64(currData.TotalExecTime, prevData.TotalExecTime),
				DeltaRows:               calcDeltaInt64(currData.Rows, prevData.Rows),
				DeltaSharedBlksHit:      calcDeltaInt64(currData.SharedBlksHit, prevData.SharedBlksHit),
				DeltaSharedBlksRead:     calcDeltaInt64(currData.SharedBlksRead, prevData.SharedBlksRead),
				DeltaSharedBlksDirtied:  calcDeltaInt64(currData.SharedBlksDirtied, prevData.SharedBlksDirtied),
				DeltaSharedBlksWritten:  calcDeltaInt64(currData.SharedBlksWritten, prevData.SharedBlksWritten),
				DeltaLocalBlksHit:       calcDeltaInt64(currData.LocalBlksHit, prevData.LocalBlksHit),
				DeltaLocalBlksRead:      calcDeltaInt64(currData.LocalBlksRead, prevData.LocalBlksRead),
				DeltaLocalBlksDirtied:   calcDeltaInt64(currData.LocalBlksDirtied, prevData.LocalBlksDirtied),
				DeltaLocalBlksWritten:   calcDeltaInt64(currData.LocalBlksWritten, prevData.LocalBlksWritten),
				DeltaTempBlksRead:       calcDeltaInt64(currData.TempBlksRead, prevData.TempBlksRead),
				DeltaTempBlksWritten:    calcDeltaInt64(currData.TempBlksWritten, prevData.TempBlksWritten),
				DeltaBlkReadTime:        calcDeltaFloat64(currData.BlkReadTime, prevData.BlkReadTime),
				DeltaBlkWriteTime:       calcDeltaFloat64(currData.BlkWriteTime, prevData.BlkWriteTime),
			})
		}
	}

	return results
}