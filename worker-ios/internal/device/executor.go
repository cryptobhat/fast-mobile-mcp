package device

import (
	"context"
	"errors"
	"sync"
)

type JobFunc func(context.Context) (any, error)

type job struct {
	ctx context.Context
	fn  JobFunc
	res chan result
}

type result struct {
	value any
	err   error
}

type Executor struct {
	jobs chan job
	wg   sync.WaitGroup
}

func NewExecutor(buffer int) *Executor {
	e := &Executor{jobs: make(chan job, buffer)}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for j := range e.jobs {
			if j.ctx.Err() != nil {
				j.res <- result{err: j.ctx.Err()}
				close(j.res)
				continue
			}
			v, err := j.fn(j.ctx)
			j.res <- result{value: v, err: err}
			close(j.res)
		}
	}()
	return e
}

func (e *Executor) Submit(ctx context.Context, fn JobFunc) (any, error) {
	resCh := make(chan result, 1)
	j := job{ctx: ctx, fn: fn, res: resCh}

	select {
	case e.jobs <- j:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case out := <-resCh:
		return out.value, out.err
	case <-ctx.Done():
		return nil, errors.Join(ctx.Err(), errors.New("executor wait cancelled"))
	}
}

func (e *Executor) Close() {
	close(e.jobs)
	e.wg.Wait()
}
