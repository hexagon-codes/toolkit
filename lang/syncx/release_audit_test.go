package syncx

import (
	"errors"
	"testing"
	"time"
)

func TestSemaphoreZeroValueAndNilContext(t *testing.T) {
	var semaphore Semaphore
	if !semaphore.TryAcquire() {
		t.Fatal("zero-value Semaphore could not acquire its default permit")
	}
	if !semaphore.TryRelease() {
		t.Fatal("zero-value Semaphore could not release its acquired permit")
	}

	if err := semaphore.AcquireContext(nil); err == nil || err.Error() != "syncx: semaphore context must not be nil" {
		t.Fatalf("AcquireContext(nil) error = %v, want a stable nil-context error", err)
	}
}

func TestConcurrentMapUpdateExcludesOtherMutations(t *testing.T) {
	values := NewConcurrentMap[string, int]()
	values.Store("count", 0)
	updateEntered := make(chan struct{})
	releaseUpdate := make(chan struct{})
	updateDone := make(chan struct{})
	go func() {
		values.Update("count", func(value int) int {
			close(updateEntered)
			<-releaseUpdate
			return value + 1
		})
		close(updateDone)
	}()
	<-updateEntered

	storeStarted := make(chan struct{})
	storeDone := make(chan struct{})
	go func() {
		close(storeStarted)
		values.Store("count", 100)
		close(storeDone)
	}()
	<-storeStarted

	select {
	case <-storeDone:
		close(releaseUpdate)
		<-updateDone
		t.Fatal("Store completed while Update held its atomic mutation boundary")
	case <-time.After(50 * time.Millisecond):
		close(releaseUpdate)
	}
	<-updateDone
	<-storeDone
	if got, _ := values.Load("count"); got != 100 {
		t.Fatalf("final value = %d, want Store to linearize after Update", got)
	}
}

func TestLazyErrErrIsRaceFreeDuringInitialization(t *testing.T) {
	sentinel := errors.New("initialization failed")
	started := make(chan struct{})
	release := make(chan struct{})
	lazy := NewLazyErr(func() (int, error) {
		close(started)
		<-release
		return 0, sentinel
	})
	getDone := make(chan struct{})
	go func() {
		_, _ = lazy.Get()
		close(getDone)
	}()
	<-started

	stopRead := make(chan struct{})
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			select {
			case <-stopRead:
				return
			default:
				_ = lazy.Err()
			}
		}
	}()
	close(release)
	<-getDone
	close(stopRead)
	<-readDone
	if !errors.Is(lazy.Err(), sentinel) {
		t.Fatalf("Err() = %v, want %v", lazy.Err(), sentinel)
	}
}
