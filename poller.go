package poller

import (
	"errors"
	"io"
	"sync"
	"time"
)

var (
	// ErrDurationTooShort occurs when calling the poller's Start
	// method with a duration that's less than 1 nanosecond.
	ErrDurationTooShort = errors.New("error: duration is less than 1ns")

	// ErrPollerRunning occurs when trying to call the poller's
	// Start method and the polling cycle is still already running
	// from previously calling Start and not yet calling Close.
	ErrPollerRunning = errors.New("error: poller is already running")
)

// Interface describes what is needed for something to be used
// as a poller.
type Interface interface {
	PollFunc(chan Eventer, chan error)
}

// An Eventer is used as a poller event.
type Eventer interface {
	Event() string
}

// A Poller turns an object that implements the poller Interface
// into a useable poller that emits events and errors when sent
// from within the implemented poller's PollFunc method.
//
// TODO: Add close select case for polling loop to be able to check
// for a case such as <-p.Close or just decide if it's simpler to
// return an ErrPollerClosed error on the <-p.Error channel.
type Poller struct {
	poller Interface
	Event  chan Eventer
	Error  chan error
	close  chan struct{}

	// mu protects running.
	mu      *sync.Mutex
	running bool
}

// New creates a new Poller for the specified Poller Interface.
func New(poller Interface) *Poller {
	return &Poller{
		poller: poller,
		Event:  make(chan Eventer),
		Error:  make(chan error),
		close:  make(chan struct{}),
		mu:     new(sync.Mutex),
	}
}

// Start begins the polling cycle which repeats every specified
// duration until Close is called.
func (p *Poller) Start(d time.Duration) error {
	// Return an error if d is less than 1 nanosecond.
	if d < time.Nanosecond {
		return ErrDurationTooShort
	}

	// Make sure the Poller is not already running.
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return ErrPollerRunning
	}
	p.running = true
	p.mu.Unlock()

	// evt and errc are passed to the poller Interface's PollFunc
	// method every polling cycle when it gets called.
	//
	// They are used instead of passing p.Event and p.Error directly
	// to the PollFunc, to help build an abstraction around things
	// such as the p.maxEvents check.
	evt, errc := make(chan Eventer), make(chan error)

	// done lets the inner polling cycle loop know when the
	// current cycle's PollFunc method has finished executing.
	done := make(chan struct{})

	for {
		go func() {
			// Start the poller Interface's PollFunc method.
			p.poller.PollFunc(evt, errc)

			// Alert the inner loop to continue to the next cycle.
			done <- struct{}{}
		}()

	inner:
		// Emit any events or errors when they occur.
		for {
			select {
			case <-p.close:
				return nil
			case err := <-errc:
				p.Error <- err
			case event := <-evt:
				p.Event <- event
			case <-done:
				break inner
			}
		}

		// Sleep for duration d before the next cycle begins.
		time.Sleep(d)
	}
}

// Close ends the Poller's Start method and if the specified Poller
// Interface implements an io.Closer, it's Close method is called,
// which can be used for polling cleanup.
func (p *Poller) Close() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = false
	p.mu.Unlock()
	p.close <- struct{}{}
	pollCloser, ok := p.poller.(io.Closer)
	if !ok {
		return nil
	}
	return pollCloser.Close()
}
