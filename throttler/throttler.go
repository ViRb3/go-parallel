package throttler

import (
	"context"
	"github.com/schollz/progressbar/v3"
	"sync"
)

type Throttler struct {
	config Config
}

type Config struct {
	// If true, continuously print a progress bar.
	ShowProgress bool
	// Use context.config.WithCancel to allow premature halt of the operation.
	Ctx context.Context
	// Size of the result channel's buffer.
	ResultBuffer int
	// How many parallel workers to run.
	Workers int
	// Data to operate on.
	Source []interface{}
	// Function that will be ran on each Source item.
	Operation func(sourceItem interface{}) interface{}
}

func NewThrottler(config Config) *Throttler {
	return &Throttler{
		config: config,
	}
}

func (t *Throttler) Run() <-chan interface{} {
	throttleChan := make(chan bool, t.config.Workers)
	resultChan := make(chan interface{}, t.config.ResultBuffer)
	wg := sync.WaitGroup{}

	bar := newBar(len(t.config.Source))
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
					bar.Add(1)
					wg.Done()
					<-throttleChan
				}()
			}
		}
		wg.Wait()
		bar.Finish()
		close(resultChan)
	}()

	return resultChan
}

func newBar(max int, description ...string) *progressbar.ProgressBar {
	bar := progressbar.Default(int64(max), description...)
	return bar
}
