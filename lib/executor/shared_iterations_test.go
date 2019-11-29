package executor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loadimpact/k6/lib/types"
	"github.com/loadimpact/k6/stats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	null "gopkg.in/guregu/null.v3"
)

func getTestSharedIterationsConfig() SharedIterationsConfig {
	return SharedIterationsConfig{
		VUs:         null.IntFrom(10),
		Iterations:  null.IntFrom(100),
		MaxDuration: types.NullDurationFrom(5 * time.Second),
	}
}

func TestSharedIterationsRun(t *testing.T) {
	t.Parallel()
	var doneIters uint64
	var ctx, cancel, executor, _ = setupExecutor(
		t, getTestSharedIterationsConfig(),
		func(ctx context.Context, out chan<- stats.SampleContainer) error {
			atomic.AddUint64(&doneIters, 1)
			return nil
		},
	)
	defer cancel()
	err := executor.Run(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(100), doneIters)
}
