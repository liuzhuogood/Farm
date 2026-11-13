package ws

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"farm/internal/action"
	"farm/internal/scrcpy"
)

func TestAutomationSDKSwipeWritesTimedIntermediatePointsAndRelease(t *testing.T) {
	var packets [][]byte
	var times []time.Time
	const pointer = uint64(1)<<62 | 42
	write := func(ctx context.Context, packet []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		packets = append(packets, packet)
		times = append(times, time.Now())
		return nil
	}
	duration := 80 * time.Millisecond
	if err := timedAutomationSwipe(context.Background(), duration, 10, 180, 90, 20,
		automationSDKPacket(pointer, 100, 200), write, nil); err != nil {
		t.Fatal(err)
	}
	if len(packets) < 5 || times[len(times)-1].Sub(times[0]) < duration {
		t.Fatalf("gesture had no meaningful duration or intermediate moves: count=%d elapsed=%v", len(packets), times[len(times)-1].Sub(times[0]))
	}
	previousX, previousY := uint32(10), uint32(180)
	for i, packet := range packets {
		if len(packet) != 32 || packet[0] != scrcpy.TypeInjectTouchEvent || binary.BigEndian.Uint64(packet[2:10]) != pointer || binary.BigEndian.Uint16(packet[18:20]) != 100 || binary.BigEndian.Uint16(packet[20:22]) != 200 {
			t.Fatalf("invalid SDK packet %d: %v", i, packet)
		}
		x, y := binary.BigEndian.Uint32(packet[10:14]), binary.BigEndian.Uint32(packet[14:18])
		if x < previousX || y > previousY {
			t.Fatalf("nonlinear/backwards movement at %d: x=%d y=%d", i, x, y)
		}
		previousX, previousY = x, y
		expected := byte(scrcpy.ActionMove)
		if i == 0 {
			expected = scrcpy.ActionDown
		}
		if i == len(packets)-1 {
			expected = scrcpy.ActionUp
			if binary.BigEndian.Uint16(packet[22:24]) != 0 || x != 90 || y != 20 {
				t.Fatalf("release not at endpoint with zero pressure: %v", packet)
			}
		}
		if packet[1] != expected {
			t.Fatalf("packet %d phase=%d expected=%d", i, packet[1], expected)
		}
	}
	if times[1].Sub(times[0]) < 5*time.Millisecond {
		t.Fatal("first movement was injected immediately instead of at its scheduled time")
	}
}

func TestAutomationHIDSwipeUsesTimedReportsWithOneContact(t *testing.T) {
	var packets [][]byte
	started := time.Now()
	err := timedAutomationSwipe(context.Background(), 80*time.Millisecond, 0, 199, 99, 0,
		automationHIDPacket(37, 100, 200), func(ctx context.Context, raw []byte) error {
			packets = append(packets, raw)
			return ctx.Err()
		}, nil)
	if err != nil || time.Since(started) < 80*time.Millisecond || len(packets) < 5 {
		t.Fatalf("HID gesture timing/count: err=%v elapsed=%v count=%d", err, time.Since(started), len(packets))
	}
	for i, packet := range packets {
		if len(packet) != 14 || packet[0] != scrcpy.TypeUhidInput || binary.BigEndian.Uint16(packet[1:3]) != scrcpy.HIDIDTouch || binary.BigEndian.Uint16(packet[3:5]) != 9 || packet[6] != 37 {
			t.Fatalf("invalid UHID packet %d: %v", i, packet)
		}
		report := packet[5:]
		if i == len(packets)-1 {
			if report[0] != 0 || report[2] != 0 || binary.LittleEndian.Uint16(report[7:9]) != 0 {
				t.Fatalf("HID contact was not released: %v", report)
			}
		} else if report[0] != 1 || report[2] != 3 || binary.LittleEndian.Uint16(report[7:9]) == 0 {
			t.Fatalf("invalid active HID contact: %v", report)
		}
	}
	first, lastMove := packets[0][5:], packets[len(packets)-2][5:]
	if binary.LittleEndian.Uint16(first[3:5]) != 0 || binary.LittleEndian.Uint16(first[5:7]) != 32767 || binary.LittleEndian.Uint16(lastMove[3:5]) != 32767 || binary.LittleEndian.Uint16(lastMove[5:7]) != 0 {
		t.Fatalf("HID endpoints were not scaled precisely: %v -> %v", first, lastMove)
	}
}

func TestAutomationSwipeCancellationReleasesOnlyItsPointerWithFreshContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var packets [][]byte
	released := false
	err := timedAutomationSwipe(ctx, time.Second, 10, 100, 80, 20,
		automationSDKPacket(73, 100, 200), func(writeCtx context.Context, packet []byte) error {
			packets = append(packets, packet)
			if packet[1] == scrcpy.ActionMove {
				cancel()
			}
			if packet[1] == scrcpy.ActionUp {
				deadline, ok := writeCtx.Deadline()
				if writeCtx.Err() != nil || !ok || time.Until(deadline) > automationTouchReleaseTimeout || binary.BigEndian.Uint64(packet[2:10]) != 73 {
					t.Errorf("release reused canceled ctx or wrong pointer: ctx=%v deadline=%v packet=%v", writeCtx.Err(), deadline, packet)
				}
				released = true
			}
			return nil
		}, nil)
	if !errors.Is(err, context.Canceled) || !released || len(packets) != 3 {
		t.Fatalf("cancel did not stop moves and release once: err=%v released=%v count=%d", err, released, len(packets))
	}
}

func TestAutomationSwipeFailuresReleaseAndPreserveOriginalError(t *testing.T) {
	for _, failPhase := range []uint8{scrcpy.ActionDown, scrcpy.ActionMove, scrcpy.ActionUp} {
		t.Run(string(rune('0'+failPhase)), func(t *testing.T) {
			original := errors.New("control write may have partially completed")
			failed, releases, downs := false, 0, 0
			err := timedAutomationSwipe(context.Background(), 32*time.Millisecond, 10, 100, 80, 20,
				automationSDKPacket(99, 100, 200), func(ctx context.Context, packet []byte) error {
					if packet[1] == scrcpy.ActionDown {
						downs++
					}
					if packet[1] == failPhase && !failed {
						failed = true
						return original
					}
					if packet[1] == scrcpy.ActionUp {
						releases++
						if ctx.Err() != nil {
							t.Error("cleanup inherited failed context")
						}
					}
					return nil
				}, nil)
			if !errors.Is(err, original) || releases != 1 || downs != 1 {
				t.Fatalf("failure was lost, gesture replayed or contact leaked: err=%v releases=%d downs=%d", err, releases, downs)
			}
		})
	}
}

func TestAutomationSwipeCleanupFailureIsReportedAndLogged(t *testing.T) {
	original, cleanup := errors.New("down failed"), errors.New("release failed")
	warned := 0
	err := timedAutomationSwipe(context.Background(), 80*time.Millisecond, 1, 2, 3, 4,
		automationHIDPacket(41, 100, 200), func(ctx context.Context, packet []byte) error {
			if packet[5] == 0 {
				return cleanup
			}
			return original
		}, func(err error) {
			if err != cleanup {
				t.Errorf("cleanup warning lost error: %v", err)
			}
			warned++
		})
	if !errors.Is(err, original) || !errors.Is(err, cleanup) || warned != 1 {
		t.Fatalf("cleanup error not preserved/reported: err=%v warnings=%d", err, warned)
	}
}

func TestAutomationSwipeRejectsCanceledOrInvalidBeforePress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx      context.Context
		duration time.Duration
	}{{ctx, time.Second}, {context.Background(), 0}, {context.Background(), 11 * time.Second}} {
		calls := 0
		err := timedAutomationSwipe(tc.ctx, tc.duration, 0, 0, 10, 20,
			automationSDKPacket(1, 100, 200), func(context.Context, []byte) error { calls++; return nil }, nil)
		if err == nil || calls != 0 {
			t.Fatalf("invalid gesture pressed contact: err=%v calls=%d", err, calls)
		}
	}
}

func TestSDKSwipeNewTimingIsRestrictedToAutomationSource(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		duration     int64
		timed        bool
	}{{"new", "automation.act", 80, true}, {"legacy", "legacy", 80, false}, {"no_duration", "automation.act", 0, false}} {
		t.Run(tc.name, func(t *testing.T) {
			device, packets := controlSocketFixture(t)
			e := deviceActionExecutor{ms: &managedSession{sess: &session{conn: device}, width: 100, height: 200}}
			started := time.Now()
			err := e.executeSDK(context.Background(), action.Request{Action: "swipe", Source: tc.source, DurationMS: tc.duration, X: .1, Y: .9, X2: .9, Y2: .1})
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for {
				select {
				case packet := <-packets:
					count++
					if packet[1] == scrcpy.ActionUp {
						if tc.timed && (count <= 3 || time.Since(started) < 80*time.Millisecond) {
							t.Fatalf("new source did not follow timed path: count=%d elapsed=%v", count, time.Since(started))
						}
						if !tc.timed && count != 3 {
							t.Fatalf("legacy gesture changed: count=%d", count)
						}
						return
					}
				case <-time.After(time.Second):
					t.Fatal("gesture left contact pressed on actual control socket")
				}
			}
		})
	}
}
