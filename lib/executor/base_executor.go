/*
 *
 * k6 - a next-generation load testing tool
 * Copyright (C) 2019 Load Impact
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package executor

import (
	"context"
	"strconv"
	"sync/atomic"

	"github.com/sirupsen/logrus"

	"github.com/loadimpact/k6/lib"
	"github.com/loadimpact/k6/stats"
	"github.com/loadimpact/k6/ui/pb"
)

// BaseExecutor is a helper struct that contains common properties and methods
// between most executors. It is intended to be used as an anonymous struct
// inside of most of the executors, for the purpose of reducing boilerplate
// code.
type BaseExecutor struct {
	config         lib.ExecutorConfig
	executionState *lib.ExecutionState
	localVUID      *uint64 // counter for assigning executor-specific VU IDs
	localIters     *uint64 // counter for keeping track of all VU iterations completed by this executor
	logger         *logrus.Entry
	progress       *pb.ProgressBar
}

// NewBaseExecutor returns an initialized BaseExecutor
func NewBaseExecutor(config lib.ExecutorConfig, es *lib.ExecutionState, logger *logrus.Entry) *BaseExecutor {
	return &BaseExecutor{
		config:         config,
		executionState: es,
		localVUID:      new(uint64),
		localIters:     new(uint64),
		logger:         logger,
		progress: pb.New(
			pb.WithLeft(config.GetName),
			pb.WithLogger(logger),
		),
	}
}

// Init doesn't do anything for most executors, since initialization of all
// planned VUs is handled by the executor.
func (bs *BaseExecutor) Init(_ context.Context) error {
	return nil
}

// GetConfig returns the configuration with which this executor was launched.
func (bs BaseExecutor) GetConfig() lib.ExecutorConfig {
	return bs.config
}

// GetNextLocalVUID increments and returns the next VU ID that's specific for
// this executor (i.e. not global like __VU).
func (bs BaseExecutor) GetNextLocalVUID() uint64 {
	return atomic.AddUint64(bs.localVUID, 1)
}

// GetLogger returns the executor logger entry.
func (bs BaseExecutor) GetLogger() *logrus.Entry {
	return bs.logger
}

// GetProgress just returns the progressbar pointer.
func (bs BaseExecutor) GetProgress() *pb.ProgressBar {
	return bs.progress
}

// getMetricTags returns a tag set that can be used to emit metrics by the
// executor. The VU ID is optional.
func (bs BaseExecutor) getMetricTags(vuID *uint64) *stats.SampleTags {
	tags := bs.executionState.Options.RunTags.CloneTags()
	if bs.executionState.Options.SystemTags.Has(stats.TagScenario) {
		tags["scenario"] = bs.config.GetName()
	}
	if vuID != nil && bs.executionState.Options.SystemTags.Has(stats.TagVU) {
		tags["vu"] = strconv.FormatUint(*vuID, 10)
	}
	return stats.IntoSampleTags(&tags)
}

// GetScenarioIter returns the completed iterations by all VUs for this executor.
func (bs *BaseExecutor) GetScenarioIter() uint64 {
	return atomic.LoadUint64(bs.localIters)
}

// IncrScenarioIter increments the counter of completed iterations by all VUs
// for this executor.
func (bs *BaseExecutor) IncrScenarioIter() {
	atomic.AddUint64(bs.localIters, 1)
}
