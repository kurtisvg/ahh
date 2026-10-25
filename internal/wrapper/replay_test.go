package wrapper

import (
	"bytes"
	"testing"
)

func TestReplayCursorAndLiveBoundary(t *testing.T) {
	terminal := newTerminal()
	_, _ = terminal.Write([]byte("before"))

	cursor := uint64(3)
	state, err := terminal.attach(&cursor)
	if err != nil || state.gap || state.history.offset != 3 || string(state.history.data) != "ore" {
		t.Fatalf("replay = %+v, state.gap = %v, err = %v", state.history, state.gap, err)
	}
	defer terminal.detach(state.client)

	// New output starts exactly where the replay snapshot ends.
	_, _ = terminal.Write([]byte("after"))
	live := <-state.client.output
	if live.offset != 6 || string(live.data) != "after" {
		t.Fatalf("live = %+v", live)
	}
}

func TestExpiredAndFutureReplayCursors(t *testing.T) {
	terminal := newTerminal()
	_, _ = terminal.Write(bytes.Repeat([]byte("a"), replayLimit))
	_, _ = terminal.Write([]byte("tail"))

	cursor := uint64(0)
	state, err := terminal.attach(&cursor)
	if err != nil || !state.gap || state.history.offset != 4 || len(state.history.data) != replayLimit {
		t.Fatalf("replay offset = %d, state.gap = %v, err = %v", state.history.offset, state.gap, err)
	}
	terminal.detach(state.client)

	cursor = terminal.nextOffset + 1
	if _, err := terminal.attach(&cursor); err == nil {
		t.Fatal("accepted future cursor")
	}
}

func TestAttachmentLimit(t *testing.T) {
	for _, exited := range []bool{false, true} {
		terminal := newTerminal()
		if exited {
			terminal.finish()
		}

		for range 16 {
			if _, err := terminal.attach(nil); err != nil {
				t.Fatal(err)
			}
		}

		if _, err := terminal.attach(nil); err == nil {
			t.Fatalf("attachment limit not enforced (exited = %v)", exited)
		}
		terminal.finish()
	}
}
