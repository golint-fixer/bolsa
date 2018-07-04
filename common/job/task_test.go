package job

import (
	"context"
	"fmt"
	"testing"

	"runtime"

	"github.com/samwooo/bolsa/common"
	"github.com/samwooo/bolsa/common/logging"
	"github.com/stretchr/testify/assert"
)

func TestTaskWithSingleWorkerWithoutBatch(t *testing.T) {
	logging.DefaultLogger(fmt.Sprintf(" < %s > ", common.APP_NAME),
		logging.LogLevelFromString("INFO"), 100)

	with := []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9}
	output := NewTask(logging.GetLogger(" task test "), "TestTaskWithoutBatch", 1, 1,
		func(ctx context.Context, d Done) Done {
			return Done{nil, d.P, nil}
		}).Run(context.Background(), NewDataSupplier(with).Adapt())

	for d := range output {
		assert.Equal(t, true, common.IsIn(d.R, with))
	}
}

func TestTaskWithNWorkersWithoutBatch(t *testing.T) {
	logging.DefaultLogger(fmt.Sprintf(" < %s > ", common.APP_NAME),
		logging.LogLevelFromString("INFO"), 100)

	with := []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9}
	output := NewTask(logging.GetLogger(" task test "), "TestTaskWithoutBatch",
		runtime.NumCPU(), 1,
		func(ctx context.Context, d Done) Done {
			return Done{nil, d.P, nil}
		}).Run(context.Background(), NewDataSupplier(with).Adapt())

	for d := range output {
		assert.Equal(t, true, common.IsIn(d.R, with))
	}
}

func TestTaskWithSingleWorkerWithBatch(t *testing.T) {
	logging.DefaultLogger(fmt.Sprintf(" < %s > ", common.APP_NAME),
		logging.LogLevelFromString("INFO"), 100)

	with := []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9}
	output := NewTask(logging.GetLogger(" task test "), "TestTaskWithoutBatch", 1, 2,
		func(ctx context.Context, d Done) Done {
			return Done{nil, d.P, nil}
		}).Run(context.Background(), NewDataSupplier(with).Adapt())

	for d := range output {
		rs, ok := d.R.([]interface{})
		assert.Equal(t, true, ok)
		for _, r := range rs {
			assert.Equal(t, true, common.IsIn(r, with))
		}
	}
}

func TestTaskWithNWorkersWithBatch(t *testing.T) {
	logging.DefaultLogger(fmt.Sprintf(" < %s > ", common.APP_NAME),
		logging.LogLevelFromString("INFO"), 100)

	with := []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9}
	output := NewTask(logging.GetLogger(" task test "), "TestTaskWithoutBatch",
		runtime.NumCPU(), 3,
		func(ctx context.Context, d Done) Done {
			return Done{nil, d.P, nil}
		}).Run(context.Background(), NewDataSupplier(with).Adapt())

	for d := range output {
		rs, ok := d.R.([]interface{})
		assert.Equal(t, true, ok)
		for _, r := range rs {
			assert.Equal(t, true, common.IsIn(r, with))
		}
	}
}