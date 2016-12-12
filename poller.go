package poller

import (
	"errors"
	"io"
	"sync"
	"time"
)

var (
	// ErrDurationTooShort occurs when calling the poller's Start
	// method with a specified duration of less than 1 ms.
	ErrDurationTooShort = errors.New("error: d is less than 1 ms")

	// ErrPollerRunning occurs when trying to call the poller's
	// Start method and the polling cycle is still already running
	// from previously calling Start and not yet calling Close.
	ErrPollerRunning = errors.New("error: poller is already running")
)

// Interface describes what is needed for something to be used
// as a poller.
type Interface interface {
	PollFunc(chan Event, chan error)
}

// Event contains a single method Type, which is the minimum method
// that needs to be implemented by an object in order to be used and
// passed around as a poller event.
type Event interface {
	Type() string
}

// A Poller turns an object that implements the poller Interface
// into a useable poller that emits events and errors when sent
// from within the implemented poller's PollFunc method.
type Poller struct {
	poller Interface
	Event  chan Event
	Error  chan error
	close  chan struct{}

	// mu protects running and maxEvents.
	mu        *sync.Mutex
	running   bool
	maxEvents int
}

// New creates a new Poller for the specified Poller Interface.
func New(poller Interface) *Poller {
	return &Poller{
		poller: poller,
		Event:  make(chan Event),
		Error:  make(chan error),
		close:  make(chan struct{}),
		mu:     new(sync.Mutex),
	}
}

// SetMaxEvents sets the maximum amount of events that can be sent
// on the poller's Event channel per watching cycle.
//
// If amount is less than 1, there is no limit, which is the default.
func (p *Poller) SetMaxEvents(amount int) {
	p.mu.Lock()
	p.maxEvents = amount
	p.mu.Unlock()
}

// Start begins the polling cycle which repeats every specified
// duration until Close is called.
func (p *Poller) Start(d time.Duration) error {
	// Give the poller a responsible polling period. Return an
	// error if d is less than 1 millisecond.
	if d < time.Millisecond {
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
	evt, errc := make(chan Event), make(chan error)

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

		// numEvents keeps track of the number of events that
		// occur per polling cycle to be used with p.maxEvents.
		numEvents := 0

	inner:
		// Emit any events or errors when they occur.
		for {
			select {
			case <-p.close:
				return nil
			case err := <-errc:
				p.Error <- err
			case event := <-evt:
				// Ignore any more events from this cycle.
				if p.maxEvents > 0 && numEvents == p.maxEvents {
					// TODO: work out proper filtering so we can
					// cancel the cycle early without the extra events
					// coming through in the next cycle.
					continue
				}
				numEvents++
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
