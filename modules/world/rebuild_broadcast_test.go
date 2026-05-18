package world

import (
	"bytes"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// TestBroadcastRebuildStaff_DeliversToInvokerAndStaffOnly pins that a
// rebuild outcome reaches invoker + every online staff (modlvl>=4) but
// NOT non-staff players. Mirrors spec §4.5.
func TestBroadcastRebuildStaff_DeliversToInvokerAndStaffOnly(t *testing.T) {
	s := newTestServer(t)

	invoker, invokerConn := newTestPlayer(t)
	invoker.client.server = s
	invoker.staffModLevel = 4
	invoker.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	staff, staffConn := newTestPlayer(t)
	staff.client.server = s
	staff.staffModLevel = 4
	staff.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	peasant, peasantConn := newTestPlayer(t)
	peasant.client.server = s
	peasant.staffModLevel = 0
	peasant.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.playersMu.Lock()
	s.playerLoop = []*Player{invoker, staff, peasant}
	s.playersMu.Unlock()

	invokerRcv := drainConn(t, invokerConn)
	staffRcv := drainConn(t, staffConn)
	peasantRcv := drainConn(t, peasantConn)

	s.broadcastRebuildStaff(invoker, "Rebuilt: 42ms.")

	invoker.client.flushWrite()
	staff.client.flushWrite()
	peasant.client.flushWrite()

	if got := <-invokerRcv; !bytes.Contains(got, []byte("Rebuilt: 42ms.")) {
		t.Errorf("invoker missing message; got %q", got)
	}
	if got := <-staffRcv; !bytes.Contains(got, []byte("Rebuilt: 42ms.")) {
		t.Errorf("staff missing message; got %q", got)
	}
	if got := <-peasantRcv; bytes.Contains(got, []byte("Rebuilt: 42ms.")) {
		t.Errorf("non-staff must NOT receive rebuild message; got %q", got)
	}
}

// TestBroadcastRebuildStaff_FsnotifyTriggered_NoInvoker pins the
// auto-rebuild path (invoker == nil): only staff receive; non-staff
// don't.
func TestBroadcastRebuildStaff_FsnotifyTriggered_NoInvoker(t *testing.T) {
	s := newTestServer(t)

	staff, staffConn := newTestPlayer(t)
	staff.client.server = s
	staff.staffModLevel = 4
	staff.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	peasant, peasantConn := newTestPlayer(t)
	peasant.client.server = s
	peasant.staffModLevel = 0
	peasant.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.playersMu.Lock()
	s.playerLoop = []*Player{staff, peasant}
	s.playersMu.Unlock()

	staffRcv := drainConn(t, staffConn)
	peasantRcv := drainConn(t, peasantConn)

	s.broadcastRebuildStaff(nil, "Rebuilding scripts...")

	staff.client.flushWrite()
	peasant.client.flushWrite()

	if got := <-staffRcv; !bytes.Contains(got, []byte("Rebuilding scripts...")) {
		t.Errorf("staff missing message; got %q", got)
	}
	if got := <-peasantRcv; bytes.Contains(got, []byte("Rebuilding scripts...")) {
		t.Errorf("non-staff must NOT receive auto-rebuild message; got %q", got)
	}
}
