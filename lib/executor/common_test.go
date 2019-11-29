package executor

import (
	"context"
	"io/ioutil"
	"math/rand"
	"testing"

	"github.com/loadimpact/k6/lib"
	"github.com/loadimpact/k6/lib/testutils"
	"github.com/loadimpact/k6/stats"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func setupExecutor(
	t *testing.T, config lib.ExecutorConfig,
	vuFn func(context.Context, chan<- stats.SampleContainer) error,
) (context.Context, context.CancelFunc, lib.Executor, *testutils.SimpleLogrusHook, *lib.ExecutionState) {
	ctx, cancel := context.WithCancel(context.Background())
	logHook := &testutils.SimpleLogrusHook{HookedLevels: []logrus.Level{logrus.WarnLevel}}
	testLog := logrus.New()
	testLog.AddHook(logHook)
	testLog.SetOutput(ioutil.Discard)
	logEntry := logrus.NewEntry(testLog)
	es := lib.NewExecutionState(lib.Options{}, 10, 50)
	runner := lib.MiniRunner{
		Fn: vuFn,
	}

	es.SetInitVUFunc(func(_ context.Context, logger *logrus.Entry) (lib.VU, error) {
		return &lib.MiniRunnerVU{R: runner, ID: rand.Int63()}, nil
	})

	segment := es.Options.ExecutionSegment
	maxVUs := lib.GetMaxPossibleVUs(config.GetExecutionRequirements(segment))
	initializeVUs(ctx, t, logEntry, es, maxVUs)

	executor, err := config.NewExecutor(es, logEntry)
	require.NoError(t, err)

	err = executor.Init(ctx)
	require.NoError(t, err)
	return ctx, cancel, executor, logHook, es
}

func initializeVUs(
	ctx context.Context, t testing.TB, logEntry *logrus.Entry, es *lib.ExecutionState, number uint64,
) {
	for i := uint64(0); i < number; i++ {
		vu, err := es.InitializeNewVU(ctx, logEntry)
		require.NoError(t, err)
		es.AddInitializedVU(vu)
	}
}
