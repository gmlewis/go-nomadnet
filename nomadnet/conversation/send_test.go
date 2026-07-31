// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package conversation

import (
	"encoding/hex"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// fakeSendDeps is a test stand-in for SendDeps. By default it uses the real
// conversation.Ingest against convPath so that Send produces an on-disk
// message and a path, exercising the full ingest path.
type fakeSendDeps struct {
	dest      *rns.Destination
	source    *rns.Destination
	preferred byte
	trust     byte
	propNode  []byte
	linkAvail bool
	ratchet   []byte
	tryProp   bool
	convPath  string

	handled      []*lxmf.Message
	ingestCount  int
	ingestOrigin []bool
}

func (f *fakeSendDeps) SendDestination() *rns.Destination { return f.dest }
func (f *fakeSendDeps) LXMFSource() *rns.Destination      { return f.source }
func (f *fakeSendDeps) PreferredDelivery([]byte) byte     { return f.preferred }
func (f *fakeSendDeps) TrustLevel([]byte) byte            { return f.trust }
func (f *fakeSendDeps) OutboundPropagationNode() []byte   { return f.propNode }
func (f *fakeSendDeps) DeliveryLinkAvailable([]byte) bool { return f.linkAvail }
func (f *fakeSendDeps) CurrentRatchetID([]byte) []byte    { return f.ratchet }
func (f *fakeSendDeps) TryPropagationOnFail() bool        { return f.tryProp }
func (f *fakeSendDeps) HandleOutbound(lxm *lxmf.Message) error {
	f.handled = append(f.handled, lxm)
	return nil
}
func (f *fakeSendDeps) Ingest(lxm *lxmf.Message, originator bool) (string, error) {
	f.ingestCount++
	f.ingestOrigin = append(f.ingestOrigin, originator)
	if f.convPath == "" {
		return "", nil
	}
	return Ingest(lxm, f.convPath, originator)
}

func newLXMFDest(t *testing.T) *rns.Destination {
	t.Helper()
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	return dest
}

// TestSendDirect verifies the default DIRECT path: a direct-preferred, link-
// available, untrusted peer yields a MethodDirect message with no ticket, the
// delivery/failed callbacks wired, the message handed to the router, and the
// outbound message ingested into the conversation.
func TestSendDirect(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	dest := newLXMFDest(t)
	source := newLXMFDest(t)

	deps := &fakeSendDeps{
		dest:      dest,
		source:    source,
		preferred: directory.DeliveryDirect,
		trust:     directory.TrustUnknown,
		linkAvail: true,
		convPath:  dir,
	}

	c := NewConversation(hex.EncodeToString(dest.Hash), dir)
	c.SetSendDeps(deps)

	if !c.Send("hello body", "a title", nil) {
		t.Fatal("Send returned false")
	}

	if len(deps.handled) != 1 {
		t.Fatalf("HandleOutbound called %d times, want 1", len(deps.handled))
	}
	lxm := deps.handled[0]
	if lxm.DesiredMethod != lxmf.MethodDirect {
		t.Errorf("DesiredMethod = %d, want MethodDirect", lxm.DesiredMethod)
	}
	if lxm.IncludeTicket {
		t.Error("IncludeTicket = true, want false for untrusted peer")
	}
	if lxm.DeliveryCallback == nil {
		t.Error("DeliveryCallback not wired")
	}
	if lxm.FailedCallback == nil {
		t.Error("FailedCallback not wired")
	}
	if string(lxm.Content) != "hello body" {
		t.Errorf("Content = %q, want %q", lxm.Content, "hello body")
	}
	if string(lxm.Title) != "a title" {
		t.Errorf("Title = %q, want %q", lxm.Title, "a title")
	}
	if deps.ingestCount != 1 {
		t.Errorf("Ingest called %d times, want 1", deps.ingestCount)
	}
	if len(deps.ingestOrigin) != 1 || !deps.ingestOrigin[0] {
		t.Error("Ingest not called with originator=true")
	}
	if len(c.Messages) != 1 {
		t.Errorf("c.Messages = %d, want 1", len(c.Messages))
	}
}

// TestSendPropagated verifies a propagated-preferred peer with a propagation
// node yields MethodPropagated and sets TryPropagationOnFail.
func TestSendPropagated(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	dest := newLXMFDest(t)
	source := newLXMFDest(t)

	deps := &fakeSendDeps{
		dest:      dest,
		source:    source,
		preferred: directory.DeliveryPropagated,
		trust:     directory.TrustUnknown,
		propNode:  []byte{0xaa, 0xbb, 0xcc},
		tryProp:   true,
		convPath:  dir,
	}

	c := NewConversation(hex.EncodeToString(dest.Hash), dir)
	c.SetSendDeps(deps)

	if !c.Send("prop body", "", nil) {
		t.Fatal("Send returned false")
	}
	lxm := deps.handled[0]
	if lxm.DesiredMethod != lxmf.MethodPropagated {
		t.Errorf("DesiredMethod = %d, want MethodPropagated", lxm.DesiredMethod)
	}
	if !lxm.TryPropagationOnFail {
		t.Error("TryPropagationOnFail = false, want true")
	}
}

// TestSendOpportunistic verifies that a direct-preferred peer with no link and
// a known ratchet yields MethodOpportunistic.
func TestSendOpportunistic(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	dest := newLXMFDest(t)
	source := newLXMFDest(t)

	deps := &fakeSendDeps{
		dest:      dest,
		source:    source,
		preferred: directory.DeliveryDirect,
		trust:     directory.TrustUnknown,
		linkAvail: false,
		ratchet:   []byte{0x01, 0x02},
		convPath:  dir,
	}

	c := NewConversation(hex.EncodeToString(dest.Hash), dir)
	c.SetSendDeps(deps)

	if !c.Send("opp body", "", nil) {
		t.Fatal("Send returned false")
	}
	lxm := deps.handled[0]
	if lxm.DesiredMethod != lxmf.MethodOpportunistic {
		t.Errorf("DesiredMethod = %d, want MethodOpportunistic", lxm.DesiredMethod)
	}
}

// TestSendTrustedIncludesTicket verifies a trusted peer gets IncludeTicket.
func TestSendTrustedIncludesTicket(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	dest := newLXMFDest(t)
	source := newLXMFDest(t)

	deps := &fakeSendDeps{
		dest:      dest,
		source:    source,
		preferred: directory.DeliveryDirect,
		trust:     directory.TrustTrusted,
		linkAvail: true,
		convPath:  dir,
	}

	c := NewConversation(hex.EncodeToString(dest.Hash), dir)
	c.SetSendDeps(deps)

	if !c.Send("trusted body", "", nil) {
		t.Fatal("Send returned false")
	}
	lxm := deps.handled[0]
	if !lxm.IncludeTicket {
		t.Error("IncludeTicket = false, want true for trusted peer")
	}
}

// TestSendNoDestination verifies Send returns false and never contacts the
// router when no send destination is configured.
func TestSendNoDestination(t *testing.T) {
	t.Parallel()

	deps := &fakeSendDeps{
		source:   newLXMFDest(t),
		convPath: tempDir(t),
	}
	c := NewConversation("deadbeef", deps.convPath)
	c.SetSendDeps(deps)

	if c.Send("body", "", nil) {
		t.Error("Send returned true with no destination")
	}
	if len(deps.handled) != 0 {
		t.Errorf("HandleOutbound called %d times, want 0", len(deps.handled))
	}
	if deps.ingestCount != 0 {
		t.Errorf("Ingest called %d times, want 0", deps.ingestCount)
	}
}

// TestMessageNotificationDeliveredIngests verifies a delivered message is
// ingested (no retry).
func TestMessageNotificationDeliveredIngests(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	dest := newLXMFDest(t)
	source := newLXMFDest(t)

	deps := &fakeSendDeps{
		dest:     dest,
		source:   source,
		convPath: dir,
	}
	c := NewConversation(hex.EncodeToString(dest.Hash), dir)
	c.SetSendDeps(deps)

	lxm, err := lxmf.NewMessage(dest, source, "body", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	lxm.State = lxmf.StateDelivered

	c.MessageNotification(lxm)

	if deps.ingestCount != 1 {
		t.Errorf("Ingest called %d times, want 1", deps.ingestCount)
	}
	if len(deps.handled) != 0 {
		t.Errorf("HandleOutbound called %d times, want 0", len(deps.handled))
	}
}

// TestMessageNotificationFailedRetriesAsPropagated verifies a failed message
// with TryPropagationOnFail set is retried as propagated and NOT ingested.
func TestMessageNotificationFailedRetriesAsPropagated(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	dest := newLXMFDest(t)
	source := newLXMFDest(t)

	deps := &fakeSendDeps{
		dest:     dest,
		source:   source,
		convPath: dir,
	}
	c := NewConversation(hex.EncodeToString(dest.Hash), dir)
	c.SetSendDeps(deps)

	lxm, err := lxmf.NewMessage(dest, source, "body", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	lxm.State = lxmf.StateFailed
	lxm.TryPropagationOnFail = true

	c.MessageNotification(lxm)

	if len(deps.handled) != 1 {
		t.Fatalf("HandleOutbound called %d times, want 1", len(deps.handled))
	}
	retried := deps.handled[0]
	if retried.DesiredMethod != lxmf.MethodPropagated {
		t.Errorf("retry DesiredMethod = %d, want MethodPropagated", retried.DesiredMethod)
	}
	if retried.TryPropagationOnFail {
		t.Error("TryPropagationOnFail still true after retry")
	}
	if deps.ingestCount != 0 {
		t.Errorf("Ingest called %d times, want 0 on retry", deps.ingestCount)
	}
}

// TestMessageNotificationFailedNoTryPropIngests verifies a failed message
// without TryPropagationOnFail is ingested (final failure recorded).
func TestMessageNotificationFailedNoTryPropIngests(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	dest := newLXMFDest(t)
	source := newLXMFDest(t)

	deps := &fakeSendDeps{
		dest:     dest,
		source:   source,
		convPath: dir,
	}
	c := NewConversation(hex.EncodeToString(dest.Hash), dir)
	c.SetSendDeps(deps)

	lxm, err := lxmf.NewMessage(dest, source, "body", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	lxm.State = lxmf.StateFailed
	lxm.TryPropagationOnFail = false

	c.MessageNotification(lxm)

	if deps.ingestCount != 1 {
		t.Errorf("Ingest called %d times, want 1", deps.ingestCount)
	}
	if len(deps.handled) != 0 {
		t.Errorf("HandleOutbound called %d times, want 0", len(deps.handled))
	}
}
