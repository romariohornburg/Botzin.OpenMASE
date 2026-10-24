package protocol_test

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"math/rand"
	"testing"

	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/testutil"
	"github.com/iagoMAO/Botzin.OpenMASE/utils"
)

func init() {
	utils.SetConfig(utils.Config{XTEA_KEY: "ea818d6680d950355f25f0fef854373e"})
}

func TestRoundTrip(t *testing.T) {
	frame := testutil.ClientFrame(byte(protocol.ShopBuyRequest), []byte{0x09, 0x2A})

	got, err := protocol.ReadFrame(bufio.NewReader(bytes.NewReader(frame)))
	if err != nil {
		t.Fatal(err)
	}

	message, err := protocol.DecryptPacket(got)
	if err != nil {
		t.Fatal(err)
	}

	if message.Type.Code() != byte(protocol.ShopBuyRequest) || !bytes.Equal(message.Payload, []byte{0x09, 0x2A}) {
		t.Fatalf("unexpected message %#v", message)
	}
}

func TestServerPacketRoundTrip(t *testing.T) {
	frame := protocol.EncryptPacket(protocol.UserDataAnswer, []byte("nick"), protocol.MASE_OK)

	message, err := protocol.DecryptPacket(frame)
	if err != nil {
		t.Fatal(err)
	}

	if message.Type.Code() != byte(protocol.UserDataAnswer) || !bytes.Equal(message.Payload, []byte{byte(protocol.MASE_OK), 0x09, 'n', 'i', 'c', 'k'}) {
		t.Fatalf("unexpected message %#v", message)
	}
}

// Several packets arriving in one TCP segment, or one packet split in many, must all be read.
func TestCoalescedAndSplitFrames(t *testing.T) {
	var stream bytes.Buffer
	for i := 0; i < 3; i++ {
		stream.Write(testutil.ClientFrame(byte(protocol.PingRequest), []byte{0x09, byte(0x21 + i)}))
	}

	reader := bufio.NewReader(&oneByteReader{data: stream.Bytes()})

	for i := 0; i < 3; i++ {
		frame, err := protocol.ReadFrame(reader)
		if err != nil {
			t.Fatalf("frame %d: %s", i, err)
		}

		message, err := protocol.DecryptPacket(frame)
		if err != nil || message.Payload[1] != byte(0x21+i) {
			t.Fatalf("frame %d: %v %#v", i, err, message)
		}
	}

	if _, err := protocol.ReadFrame(reader); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

type oneByteReader struct{ data []byte }

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

func TestInvalidLengthHeader(t *testing.T) {
	_, err := protocol.ReadFrame(bufio.NewReader(bytes.NewReader([]byte{0xFF, 0xFF, 1, 2, 3})))
	if !errors.Is(err, protocol.ErrMalformed) {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
}

// Random garbage must produce errors, never panics.
func TestDecryptGarbage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	for i := 0; i < 5000; i++ {
		frame := make([]byte, rng.Intn(64))
		rng.Read(frame)
		protocol.DecryptPacket(frame)
	}
}

func TestFields(t *testing.T) {
	message := protocol.NetworkMessage{Payload: testutil.Request([]byte{0x21}, []byte("abc"))}

	if message.IntField(0) != 1 || string(message.Field(1)) != "abc" || message.Field(5) != nil {
		t.Fatalf("unexpected fields %q", message.Fields())
	}

	if (protocol.NetworkMessage{}).Field(0) != nil {
		t.Fatal("empty payload should have no fields")
	}
}
