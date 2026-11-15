// SPDX-License-Identifier: MIT

package cmd

import (
	"sync/atomic"
	"time"

	"github.com/otfabric/knxctl/internal/output"
)

// The telegram records of serve wait for stdout in a queue of serveQueue
// records; after Ctrl-C, serve waits at most serveDrainTimeout for the
// queue to be written. Variables for the tests.
var (
	serveQueue        = 1024
	serveDrainTimeout = 2 * time.Second
)

// printer writes the telegram records of serve on a goroutine of its own:
// go-knx calls the Subscribe function on the server's goroutines, which it
// must not block, so a stdout that does not take the output (a pager that
// stopped reading) must not hold up the server. A record that finds the
// queue full is dropped and counted; the next record queued carries the
// count in its dropped field.
type printer struct {
	st      *output.Stream
	records chan telegramRecord
	stop    chan struct{} // closed by close: write what is queued, then end
	done    chan struct{} // closed when the writer has ended

	pending   atomic.Int64 // dropped since the last record queued
	dropped   atomic.Int64 // dropped in all
	queued    atomic.Int64 // records queued
	written   atomic.Int64 // records written
	abandoned atomic.Bool  // close gave up waiting: write nothing more
}

func newPrinter(st *output.Stream, queue int) *printer {
	p := &printer{st: st, records: make(chan telegramRecord, queue), stop: make(chan struct{}), done: make(chan struct{})}
	go p.run()
	return p
}

// add queues r without blocking. Several goroutines may call it at once.
func (p *printer) add(r telegramRecord) {
	r.Dropped = p.pending.Swap(0)
	select {
	case p.records <- r:
		p.queued.Add(1)
	default:
		p.pending.Add(r.Dropped + 1)
		p.dropped.Add(1)
	}
}

func (p *printer) run() {
	defer close(p.done)
	for {
		var r telegramRecord
		select {
		case r = <-p.records:
		case <-p.stop:
			select {
			case r = <-p.records:
			default:
				return
			}
		}
		if p.abandoned.Load() {
			return
		}
		_ = p.st.Record("telegram", r.line(), r)
		p.written.Add(1)
	}
}

// close writes the queued records, waiting at most timeout, and returns
// the number of telegrams not written: dropped, or still queued when the
// wait ended. Call it once no more records are added.
func (p *printer) close(timeout time.Duration) int64 {
	close(p.stop)
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-p.done:
	case <-t.C:
		p.abandoned.Store(true)
	}
	return p.dropped.Load() + p.queued.Load() - p.written.Load()
}
