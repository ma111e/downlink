package main

import (
	"io"

	"github.com/ma111e/downlink/cmd/server/notification"

	log "github.com/sirupsen/logrus"
)

// cliProgress adapts the batchProgress spinner to the notification package's
// PublishProgress sink so publish/republish operations render live steps.
type cliProgress struct {
	d *batchProgress
}

func (c *cliProgress) Start(step, label string)  { c.d.addRow(step, label) }
func (c *cliProgress) Update(step, label string) { c.d.updateRow(step, label) }
func (c *cliProgress) Complete(step string, ok bool, note string) {
	c.d.completeRow(step, ok, note)
}

// runPublishWithProgress runs fn under a live spinner, wiring a cliProgress sink
// into the publisher and silencing logrus for the duration so its lines don't
// corrupt the in-place redraw. The progress sink is also handed to fn so the
// caller can emit its own steps (e.g. fetching digests before publishing).
func runPublishWithProgress(p *notification.GitHubPagesPublisher,
	fn func(prog notification.PublishProgress) error) error {
	d := newBatchProgress()
	prev := log.StandardLogger().Out
	log.SetOutput(io.Discard)
	defer log.SetOutput(prev)

	prog := &cliProgress{d: d}
	p.SetProgress(prog)
	d.startSpinner()
	defer d.stop()

	return fn(prog)
}
