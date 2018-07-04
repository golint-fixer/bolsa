package job

import (
	"context"
	"fmt"
	"runtime"

	"github.com/samwooo/bolsa/common/logging"
)

func (j *Job) drain(ctx context.Context, s supplier) <-chan Done {
	return NewTask(j.Logger, "drain", j.workers,
		func() int {
			if j.batchStrategy == nil {
				return 1
			} else {
				return j.batchStrategy.Size()
			}
		}(),
		func(ctx context.Context, d Done) Done {
			j.Logger.Debugf("√ drain succeed ( %+v )", d)
			return Done{nil, d.P, nil}
		}).Run(ctx, s.Adapt())
}

func (j *Job) feed(ctx context.Context, input <-chan Done) <-chan Done {

	type reducer func(context.Context, []interface{}) (interface{}, error)

	batchSize := func() int {
		if j.batchStrategy == nil {
			return 1
		} else {
			return j.batchStrategy.Size()
		}
	}()

	feedWithReduce := func(workers int, input <-chan Done, reduce reducer) <-chan Done {
		return NewTask(j.Logger, "reduce", workers, 1,
			func(ctx context.Context, d Done) Done {
				var batchedData []interface{}
				if batchSize <= 1 {
					batchedData = append(batchedData, d.P)
				} else {
					batchedData, _ = d.P.([]interface{})
				}
				if data, err := reduce(ctx, batchedData); err != nil {
					j.Logger.Errorf("× reduce failed ( %+v, %s )", batchedData, err.Error())
					return Done{
						batchedData,
						nil,
						newError(typeBatch, fmt.Errorf("( %+v, %s )", batchedData, err.Error()))}
				} else {
					j.Logger.Debugf("√ reduce succeed ( %+v, %+v )", batchedData, data)
					return Done{batchedData, data, nil}
				}
			}).Run(ctx, input)
	}

	if j.batchStrategy != nil {
		j.Logger.Debugf("❋ reduce √ ( size %d )", j.batchStrategy.Size())
		return feedWithReduce(j.workers, input, j.batchStrategy.Reduce)
	} else {
		j.Logger.Debugf("❋ reduce √ ( default )")
		return feedWithReduce(j.workers, input,
			func(ctx context.Context, batchedData []interface{}) (interface{}, error) {
				return batchedData[0], nil
			})
	}
}

func (j *Job) chew(ctx context.Context, input <-chan Done) <-chan Done {

	type worker func(ctx context.Context, p interface{}) (r interface{}, e error)

	chewWithLabor := func(workers int, input <-chan Done, work worker) <-chan Done {
		return NewTask(j.Logger, "labor", workers, 1,
			func(ctx context.Context, d Done) Done {
				if d.E != nil {
					j.Logger.Debugf("√ labor skipped, pipe batch failure ( %s )", d.E.Error())
					return d
				} else {
					if data, err := work(ctx, d.P); err != nil {
						j.Logger.Errorf("× labor failed ( %+v, %s )", d.P, err.Error())
						return Done{
							d.P,
							data,
							newError(typeLabor, fmt.Errorf("( %+v, %s )", d.P, err.Error()))}
					} else {
						j.Logger.Debugf("√ labor succeed ( %+v, %+v)", d.P, data)
						return Done{d.P, data, nil}
					}
				}
			}).Run(ctx, input)
	}

	if j.laborStrategy != nil {
		j.Logger.Debugf("❋ labor √ ( workers %d )", j.workers)
		return chewWithLabor(j.workers, input, j.laborStrategy.Work)
	} else {
		j.Logger.Debugf("❋ labor √ ( default )")
		return chewWithLabor(j.workers, input,
			func(ctx context.Context, para interface{}) (interface{}, error) {
				return para, nil
			})
	}
}

func (j *Job) digest(ctx context.Context, input <-chan Done) <-chan []Done {
	out := make(chan []Done)
	go func() {
		var results []Done
		for r := range input {
			if r.E != nil {
				if j.errorStrategy != nil {
					j.errorStrategy.OnError(r)
				}
			}
			results = append(results, r)
		}
		out <- results
		close(out)
	}()
	return out
}

func (j *Job) runWithSupplier(ctx context.Context, s supplier) []Done {
	return <-j.digest(ctx, j.chew(ctx, j.feed(ctx, j.drain(ctx, s))))
}

func (j *Job) retry(ctx context.Context, allDone []Done) []Done {
	runJobWithData := func(ctx context.Context, j *Job, data []interface{}) []Done {
		return j.runWithSupplier(ctx, NewDataSupplier(data))
	}

	var finalDone []Done
	retries := 1
	for {
		var batchRetries []interface{}
		var batchFailed []Done
		var laborRetries []interface{}
		var laborFailed []Done
		for _, done := range allDone {
			// not worth retrying
			if !j.retryStrategy.Worth(done) {
				finalDone = append(finalDone, done)
			} else {
				if e, ok := done.E.(*Error); ok {
					switch e.st {
					case typeBatch:
						if groupedPara, isArray := done.P.([]interface{}); isArray {
							batchRetries = append(batchRetries, groupedPara...)
							for _, para := range groupedPara {
								batchFailed = append(batchFailed, Done{para, nil, done.E})
							}
						} else {
							j.Logger.Errorf("× cast ( %+v ) failed, skip retry", done.P)
						}
					case typeLabor:
						laborRetries = append(laborRetries, done.P)
						laborFailed = append(laborFailed, done)
					default:
						j.Logger.Warnf("× retry unknown failure ( %+v, %s ) ?", e.st, done.E.Error())
					}
				} else {
					finalDone = append(finalDone, done)
				}
			}
		}

		if j.retryStrategy.Forgo() {
			if len(batchFailed) > 0 {
				finalDone = append(finalDone, batchFailed...)
			}
			if len(laborFailed) > 0 {
				finalDone = append(finalDone, laborFailed...)
			}
			j.Logger.Debugf("√ retry ended ( %d batch failures, %d labor failures )",
				len(batchFailed), len(laborFailed))
			break
		} else {
			allDone = []Done{}
			if len(laborRetries) > 0 {
				j.Logger.Debugf("√ retry ( %d ) on ( %d labor failures )", retries, len(laborRetries))
				// Retry labor failures in a New Job
				allDone = append(allDone, runJobWithData(ctx, NewJob(j.Logger, j.workers).LaborStrategy(
					j.laborStrategy), laborRetries)...)
			}
			if len(batchRetries) > 0 {
				j.Logger.Debugf("√ retry ( %d ) on ( %d batch failures )", retries, len(batchRetries))
				// Retry batch failures in a New Job
				allDone = append(allDone, runJobWithData(ctx, NewJob(j.Logger, j.workers).BatchStrategy(
					j.batchStrategy).LaborStrategy(j.laborStrategy), batchRetries)...)
			}
			retries++
		}
	}
	return finalDone
}

func (j *Job) BatchStrategy(bh batchStrategy) *Job {
	j.batchStrategy = bh
	return j
}

func (j *Job) LaborStrategy(lh laborStrategy) *Job {
	j.laborStrategy = lh
	return j
}

func (j *Job) RetryStrategy(rh retryStrategy) *Job {
	j.retryStrategy = rh
	return j
}

func (j *Job) ErrorStrategy(eh errorStrategy) *Job {
	j.errorStrategy = eh
	return j
}

func (j *Job) Run(ctx context.Context, s supplier) []Done {
	child, cancelFn := context.WithCancel(ctx)
	defer cancelFn()

	allDone := j.runWithSupplier(child, s)
	if j.retryStrategy == nil {
		j.Logger.Debug("❋ retry ×")
	} else {
		j.Logger.Debug("❋ retry √")
		allDone = j.retry(ctx, allDone)
	}
	j.Logger.Debugf("√ finished with ( %+v )", allDone)
	return allDone
}

func NewJob(logger logging.Logger, workers int) *Job {
	if workers <= 0 {
		workers = runtime.NumCPU() * 64
	}
	j := &Job{logger, workers,
		nil, nil,
		nil, nil}
	return j
}
