package scanner

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProgressConsumerStopWaitsForCallbackAndPreventsLaterCallbacks(t *testing.T) {
	progress := make(chan int, 2)
	stop := make(chan struct{})
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	var callbackCount atomic.Int32
	var consumer sync.WaitGroup
	consumer.Add(1)
	go func() {
		defer consumer.Done()
		for {
			select {
			case _, ok := <-progress:
				if !ok {
					return
				}
				if callbackCount.Add(1) == 1 {
					close(callbackStarted)
					<-releaseCallback
				}
			case <-stop:
				return
			}
		}
	}()
	progress <- 1
	<-callbackStarted

	stopped := make(chan struct{})
	go func() {
		stopProgressConsumer(stop, &consumer)
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("stop returned while an onProgress callback was still running")
	case <-time.After(10 * time.Millisecond):
	}
	close(releaseCallback)
	<-stopped
	if got := callbackCount.Load(); got != 1 {
		t.Fatalf("callback count after stop = %d, want 1", got)
	}
}
