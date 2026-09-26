package signaling

import (
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func TestExtractEncFromParticipant(t *testing.T) {
	device0JID, _ := types.ParseJID("169939356889212:0@lid")
	device15JID, _ := types.ParseJID("169939356889212:15@lid")
	baseJID, _ := types.ParseJID("169939356889212@lid")

	nodes := []waBinary.Node{
		{
			Tag:   "to",
			Attrs: waBinary.Attrs{"jid": device0JID.String()},
			Content: []waBinary.Node{
				{Tag: "enc", Attrs: waBinary.Attrs{"type": "msg", "v": "2"}, Content: []byte("enc_for_device_0")},
			},
		},
		{
			Tag:   "to",
			Attrs: waBinary.Attrs{"jid": device15JID.String()},
			Content: []waBinary.Node{
				{Tag: "enc", Attrs: waBinary.Attrs{"type": "pkmsg", "v": "2"}, Content: []byte("enc_for_device_15")},
			},
		},
	}

	enc15 := extractEncFromParticipant(nodes, device15JID)
	if enc15 == nil {
		t.Fatalf("expected enc node for device 15, got nil")
	}
	if string(enc15.Content.([]byte)) != "enc_for_device_15" {
		t.Errorf("got content %q, want enc_for_device_15", string(enc15.Content.([]byte)))
	}

	// Base JID query should prefer companion phone device 15 over device 0
	encBase := extractEncFromParticipant(nodes, baseJID)
	if encBase == nil {
		t.Fatalf("expected enc node for base JID, got nil")
	}
	if string(encBase.Content.([]byte)) != "enc_for_device_15" {
		t.Errorf("got content %q, want enc_for_device_15", string(encBase.Content.([]byte)))
	}
}
