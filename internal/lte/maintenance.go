package lte

import (
	"context"
	"errors"
	"fmt"

	"github.com/addxemmm/lte-system/internal/sysop"
)

// ErrCellMustBeStopped is returned when a stopped-only mutation races with,
// or is attempted during, an LTE lifecycle. Callers may use errors.Is to map
// it to a stable conflict response.
var ErrCellMustBeStopped = errors.New("cell must be stopped")

// RunWhileStopped serializes a stopped-only operation with Start and Stop.
// fn executes while the Manager lifecycle mutex is held; callbacks that also
// mutate subscribers must acquire subscriber.Mutex inside fn, preserving the
// global lock order Manager.mu -> subscriber.Mutex used by Start/renderAll.
// fn must not call Manager lifecycle methods itself.
func (m *Manager) RunWhileStopped(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.hasLifecycleLocked() {
		return fmt.Errorf("%w: manager-owned lifecycle or network resources are active", ErrCellMustBeStopped)
	}
	if sysop.Running("srsepc") || sysop.Running("srsenb") {
		return fmt.Errorf("%w: external LTE process is active", ErrCellMustBeStopped)
	}
	if fn == nil {
		return errors.New("stopped callback is nil")
	}
	return fn()
}
