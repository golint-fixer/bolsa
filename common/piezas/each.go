package piezas

import (
	"context"
	"time"

	"github.com/samwooo/bolsa/common/job"
	"github.com/samwooo/bolsa/common/logging"
)

type eachJ struct {
	*job.Job
	iterator func(interface{}) (interface{}, error)
}

func (ea *eachJ) act(ctx context.Context, p interface{}) (r interface{}, e error) {
	if ea.iterator != nil {
		return ea.iterator(p)
	} else {
		return p, nil
	}
}

func Each(ctx context.Context, logger logging.Logger, data []interface{},
	ite func(interface{}) (interface{}, error)) []job.Done {

	start := time.Now()
	e := &eachJ{job.NewJob(logger, 0), ite}
	done := e.ActionHandler(e).Run(ctx, data)
	e.Logger.Infof("done in %+v with %+v", time.Since(start), done)
	return done
}