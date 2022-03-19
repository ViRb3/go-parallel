package throttler

import (
	"context"
	"testing"
)

func TestDry(t *testing.T) {
	for p := range NewThrottler(Config[bool, bool]{
		ShowProgress: false,
		Ctx:          context.Background(),
		ResultBuffer: 1,
		Workers:      1,
		Source:       []bool{true, false, true},
		Operation: func(a bool) bool {
			return a
		},
	}).Run() {
		print(p)
	}
}
