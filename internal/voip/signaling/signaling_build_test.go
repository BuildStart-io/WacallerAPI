package signaling

import (
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func TestExtractEncFromParticipant(t *testing.T) {
	device0JID, _ := types.ParseJID("169939356889212:0@lid")
	device70JID, _ := types.ParseJID("169939356889212:70@lid")

	nodes := []waBinary.Node{
		{
			Tag:   "to",
			Attrs: waBinary.Attrs{"jid": device0JID.String()},
			Content: []waBinary.Node{
				{Tag: "enc", Attrs: waBinary.Attrs{"type": "pkmsg", "v": "2"}, Content: []byte("enc_for_device_0")},
			},
		},
		{
			Tag:   "to",
			Attrs: waBinary.Attrs{"jid": device70JID.String()},
			Content: []waBinary.Node{
				{Tag: "enc", Attrs: waBinary.Attrs{"type": "msg", "v": "2"}, Content: []byte("enc_for_device_70")},
			},
		},
	}

	enc70 := extractEncFromParticipant(nodes, device70JID)
	if enc70 == nil {
		t.Fatalf("expected enc node for device 70, got nil")
	}
	if string(enc70.Content.([]byte)) != "enc_for_device_70" {
		t.Errorf("got content %q, want enc_for_device_70", string(enc70.Content.([]byte)))
	}

	enc0 := extractEncFromParticipant(nodes, device0JID)
	if enc0 == nil {
		t.Fatalf("expected enc node for device 0, got nil")
	}
	if string(enc0.Content.([]byte)) != "enc_for_device_0" {
		t.Errorf("got content %q, want enc_for_device_0", string(enc0.Content.([]byte)))
	}
}
