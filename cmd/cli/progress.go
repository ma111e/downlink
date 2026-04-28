package main

import (
	"fmt"
	"sync"
	"time"
)

type batchRow struct {
	label   string
	done    bool
	success bool
	summary string
	hidden  bool
}

type batchProgress struct {
	rows       []*batchRow
	index      map[string]int
	frame      int
	drawnLines int
	mu         sync.Mutex
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func newBatchProgress() *batchProgress {
	return &batchProgress{
		index:  make(map[string]int),
		stopCh: make(chan struct{}),
	}
}

func (d *batchProgress) addHiddenRow(id, label string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.index[id]; exists {
		return
	}
	d.index[id] = len(d.rows)
	d.rows = append(d.rows, &batchRow{label: label, hidden: true})
}

func (d *batchProgress) showRow(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if idx, ok := d.index[id]; ok {
		d.rows[idx].hidden = false
	}
	d.redraw()
}

func (d *batchProgress) addRow(id, label string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.index[id]; !exists {
		d.index[id] = len(d.rows)
		d.rows = append(d.rows, &batchRow{label: label})
	} else {
		d.rows[d.index[id]].label = label
	}
	d.redraw()
}

func (d *batchProgress) completeRow(id string, success bool, summary string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if idx, ok := d.index[id]; ok {
		r := d.rows[idx]
		r.hidden = false
		r.done = true
		r.success = success
		r.summary = summary
	}
	d.redraw()
}

func (d *batchProgress) startSpinner() {
	ticker := time.NewTicker(100 * time.Millisecond)
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer ticker.Stop()
		for {
			select {
			case <-d.stopCh:
				return
			case <-ticker.C:
				d.mu.Lock()
				d.frame = (d.frame + 1) % len(spinnerFrames)
				d.redraw()
				d.mu.Unlock()
			}
		}
	}()
}

// redraw redraws all rows in-place. Caller must hold d.mu.
func (d *batchProgress) redraw() {
	visible := 0
	for _, r := range d.rows {
		if !r.hidden {
			visible++
		}
	}
	if visible == 0 {
		return
	}
	if d.drawnLines > 0 {
		fmt.Printf("\033[%dA", d.drawnLines)
	}
	for _, r := range d.rows {
		if r.hidden {
			continue
		}
		fmt.Print("\r\033[K")
		if r.done {
			if r.success {
				fmt.Printf("  ✓ %s %s\n", r.label, r.summary)
			} else {
				fmt.Printf("  ✗ %s %s\n", r.label, r.summary)
			}
		} else {
			fmt.Printf("  %s %s\n", spinnerFrames[d.frame], r.label)
		}
	}
	d.drawnLines = visible
}

// insertRowAfter inserts a new row immediately after the row with afterId.
// If id already exists, updates its label in-place. Falls back to append if afterId not found.
func (d *batchProgress) insertRowAfter(afterId, id, label string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.index[id]; exists {
		d.rows[d.index[id]].label = label
		d.redraw()
		return
	}
	afterIdx, ok := d.index[afterId]
	insertAt := len(d.rows)
	if ok {
		insertAt = afterIdx + 1
	}
	newRow := &batchRow{label: label}
	d.rows = append(d.rows, nil)
	copy(d.rows[insertAt+1:], d.rows[insertAt:])
	d.rows[insertAt] = newRow
	newIndex := make(map[string]int, len(d.index)+1)
	for k, v := range d.index {
		if v >= insertAt {
			newIndex[k] = v + 1
		} else {
			newIndex[k] = v
		}
	}
	newIndex[id] = insertAt
	d.index = newIndex
	d.redraw()
}

func (d *batchProgress) updateRow(id, label string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if idx, ok := d.index[id]; ok {
		d.rows[idx].label = label
	} else {
		d.index[id] = len(d.rows)
		d.rows = append(d.rows, &batchRow{label: label})
	}
	d.redraw()
}

func (d *batchProgress) resetRow(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if idx, ok := d.index[id]; ok {
		r := d.rows[idx]
		r.done = false
		r.success = false
		r.summary = ""
	}
	d.redraw()
}

func (d *batchProgress) stop() {
	close(d.stopCh)
	d.wg.Wait()
	d.mu.Lock()
	d.redraw()
	d.mu.Unlock()
}
