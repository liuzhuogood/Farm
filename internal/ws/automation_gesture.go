package ws

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"sync/atomic"
	"time"

	"farm/internal/action"
	"farm/internal/scrcpy"
)

const automationTouchReleaseTimeout = time.Second

var automationPointerSequence atomic.Uint64
var automationHIDSequence atomic.Uint32

type automationGestureWrite func(context.Context, []byte) error
type automationGesturePacket func(uint8, int32, int32) []byte

// timedAutomationSwipe is used exclusively by the new automation.act source.
// The caller holds controlMu for the entire gesture and release attempt.
// Scheduling against absolute deadlines avoids adding write time to every step.
func timedAutomationSwipe(ctx context.Context, duration time.Duration, x1, y1, x2, y2 int32,
	packet automationGesturePacket, write automationGestureWrite, cleanupWarning func(error),
) (resultErr error) {
	if duration <= 0 || duration > 10*time.Second {
		return fmt.Errorf("automation swipe duration must be in 1..10000ms")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lastX, lastY := x1, y1
	// A failed socket write may have delivered a complete or partial down.
	// From this point every unsuccessful exit must release only this contact.
	contactMayBeDown := true
	defer func() {
		if !contactMayBeDown {
			return
		}
		releaseCtx, cancel := context.WithTimeout(context.Background(), automationTouchReleaseTimeout)
		defer cancel()
		if err := write(releaseCtx, packet(scrcpy.ActionUp, lastX, lastY)); err != nil {
			if cleanupWarning != nil {
				cleanupWarning(err)
			}
			resultErr = errors.Join(resultErr, fmt.Errorf("automation touch release failed: %w", err))
		}
	}()
	if err := write(ctx, packet(scrcpy.ActionDown, x1, y1)); err != nil {
		return err
	}
	started := time.Now()
	steps := int((duration + 16*time.Millisecond - 1) / (16 * time.Millisecond))
	if steps < 2 {
		steps = 2
	}
	for i := 1; i <= steps; i++ {
		deadline := started.Add(time.Duration(int64(duration) * int64(i) / int64(steps)))
		if delay := time.Until(deadline); delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		fraction := float64(i) / float64(steps)
		lastX = int32(math.Round(float64(x1) + float64(x2-x1)*fraction))
		lastY = int32(math.Round(float64(y1) + float64(y2-y1)*fraction))
		if err := write(ctx, packet(scrcpy.ActionMove, lastX, lastY)); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := write(ctx, packet(scrcpy.ActionUp, x2, y2)); err != nil {
		return err
	}
	contactMayBeDown = false
	return nil
}

func automationSDKPacket(pointer uint64, w, h uint16) automationGesturePacket {
	return func(phase uint8, x, y int32) []byte {
		if phase == scrcpy.ActionUp {
			return scrcpy.TouchEvent(phase, pointer, x, y, w, h, 0, 0, 0)
		}
		return scrcpy.TouchEvent(phase, pointer, x, y, w, h, 1, 0, 0)
	}
}

func automationHIDPacket(id uint8, w, h uint16) automationGesturePacket {
	return func(phase uint8, x, y int32) []byte {
		if phase == scrcpy.ActionUp {
			return scrcpy.UhidInput(scrcpy.HIDIDTouch, hidTouchReport(id, 0, false, 0, 0, 0))
		}
		return scrcpy.UhidInput(scrcpy.HIDIDTouch, hidTouchReport(id, 1, true, hidScaleX(x, w), hidScaleY(y, h), 512))
	}
}

func (e deviceActionExecutor) automationSwipe(ctx context.Context, r action.Request, x1, y1, x2, y2 int32, w, h uint16, hid bool) error {
	if r.DurationMS <= 0 || r.DurationMS > 10000 {
		return fmt.Errorf("automation swipe duration must be in 1..10000ms")
	}
	if w == 0 || h == 0 {
		return fmt.Errorf("automation swipe requires current screen dimensions")
	}
	write := e.ms.sess.conn.WriteControlContext
	mode := action.ModeSDK
	pointer := uint64(1)<<62 | automationPointerSequence.Add(1)
	packet := automationSDKPacket(pointer, w, h)
	if hid {
		mode = action.ModeHID
		// Only the new source uses a cancellable creation write; the old
		// hidEnsureDevice and its callers retain their existing behavior.
		if !e.ms.hidTouchReady {
			if err := write(ctx, scrcpy.UhidCreate(scrcpy.HIDIDTouch, 0x18D1, 0xFFFF, "scrcpy-touch", hidTouchReportDesc)); err != nil {
				return err
			}
			e.ms.hidTouchReady = true
		}
		packet = automationHIDPacket(uint8(automationHIDSequence.Add(1)), w, h)
	}
	started := time.Now()
	err := timedAutomationSwipe(ctx, time.Duration(r.DurationMS)*time.Millisecond, x1, y1, x2, y2, packet, write, func(err error) {
		log.Printf("[AutomationGesture] touch release failed serial=%q mode=%s error=%v", r.DeviceID, mode, err)
	})
	log.Printf("[AutomationGesture] swipe serial=%q mode=%s requested_ms=%d elapsed_ms=%d success=%t", r.DeviceID, mode, r.DurationMS, time.Since(started).Milliseconds(), err == nil)
	return err
}
