package throttler

import (
	"context"
	"github.com/schollz/progressbar/v3"
	"sync"
)

type Throttler[T any, P any] struct {
	config Config[T, P]
}

type Config[T any, P any] struct {
	// If true, continuously print a progress bar.
	ShowProgress bool
	// Use context.config.WithCancel to allow premature halt of the operation.
	Ctx context.Context
	// Size of the result channel's buffer.
	ResultBuffer int
	// How many parallel workers to run.
	Workers int
	// Data to operate on.
	Source []T
	// Function that will be run on each Source item.
	Operation func(sourceItem T) P
}

func NewThrottler[T any, P any](config Config[T, P]) *Throttler[T, P] {
	return &Throttler[T, P]{
		config: config,
	}
}

func (t *Throttler[T, P]) Run() <-chan P {
	throttleChan := make(chan bool, t.config.Workers)
	resultChan := make(chan P, t.config.ResultBuffer)
	wg := sync.WaitGroup{}

	var bar interface{}
	if t.config.ShowProgress {
		bar = newBar(len(t.config.Source))
	}
	wg.Add(len(t.config.Source))

	go func() {
		for i := range t.config.Source {
			if t.config.Ctx.Err() != nil {
				wg.Done()
			} else {
				sourceItem := t.config.Source[i]
				throttleChan <- true
				go func() {
					result := t.config.Operation(sourceItem)
					resultChan <- result
					if t.config.ShowProgress {
						bar.(*progressbar.ProgressBar).Add(1)
					}
					wg.Done()
					<-throttleChan
				}()
			}
		}
		wg.Wait()
		if t.config.ShowProgress {
			bar.(*progressbar.ProgressBar).Finish()
		}
		close(resultChan)
	}()

	return resultChan
}

func newBar(max int, description ...string) *progressbar.ProgressBar {
	bar := progressbar.Default(int64(max), description...)
	return bar
}
