package executor

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loadimpact/k6/lib/types"
	"github.com/loadimpact/k6/stats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	null "gopkg.in/guregu/null.v3"
)

func getTestVariableLoopingVUsConfig() VariableLoopingVUsConfig {
	return VariableLoopingVUsConfig{
		BaseConfig: BaseConfig{GracefulStop: types.NullDurationFrom(0)},
		StartVUs:   null.IntFrom(5),
		Stages: []Stage{
			{
				Duration: types.NullDurationFrom(1 * time.Second),
				Target:   null.IntFrom(5),
			},
			{
				Duration: types.NullDurationFrom(0),
				Target:   null.IntFrom(3),
			},
			{
				Duration: types.NullDurationFrom(1 * time.Second),
				Target:   null.IntFrom(3),
			},
		},
		GracefulRampDown: types.NullDurationFrom(0),
	}
}

func TestVariableLoopingVUsRun(t *testing.T) {
	t.Parallel()
	var iterCount int64
	var ctx, cancel, executor, _, es = setupExecutor(
		t, getTestVariableLoopingVUsConfig(),
		func(ctx context.Context, out chan<- stats.SampleContainer) error {
			time.Sleep(200 * time.Millisecond)
			atomic.AddInt64(&iterCount, 1)
			return nil
		},
	)
	defer cancel()
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		result []int64
	)
	addResult := func(currActiveVUs int64) {
		mu.Lock()
		result = append(result, currActiveVUs)
		mu.Unlock()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		addResult(es.GetCurrentlyActiveVUsCount())
		time.Sleep(1 * time.Second)
		addResult(es.GetCurrentlyActiveVUsCount())
		time.Sleep(1 * time.Second)
		addResult(es.GetCurrentlyActiveVUsCount())
	}()
	err := executor.Run(ctx, nil)
	wg.Wait()
	require.NoError(t, err)
	assert.Equal(t, []int64{5, 3, 0}, result)
	assert.Equal(t, int64(40), iterCount)
}
