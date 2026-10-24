package protocol

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/iagoMAO/Botzin.OpenMASE/security"
	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"
	"github.com/rs/zerolog/log"
)

// Largest encrypted frame we accept from a client. Real packets are far smaller.
const MaxFrameLength = 8192

const md5Length = 16

var (
	ErrFrameTooLarge = errors.New("frame too large")
	ErrMalformed     = errors.New("malformed packet")
)

type NetworkMessage struct {
	Type    PacketCode
	Payload []byte
}

// Fields splits the payload (skipping its first byte) on tabs, the way the client
// serializes its requests.
func (m NetworkMessage) Fields() [][]byte {
	if len(m.Payload) < 1 {
		return nil
	}
	return bytes.Split(m.Payload[1:], []byte{'\t'})
}

// Field returns the field at index i or nil when it doesn't exist.
func (m NetworkMessage) Field(i int) []byte {
	fields := m.Fields()
	if i < 0 || i >= len(fields) {
		return nil
	}
	return fields[i]
}

// IntField returns the SCR packed integer at index i (0 when missing).
func (m NetworkMessage) IntField(i int) int {
	return data.SCR_StrToInt(m.Field(i))
}

type Packet interface {
	Compose() []byte
}

type PacketCode interface {
	Code() byte
}

// ReadFrame reads a single length prefixed frame from the stream.
// The prefix is the length of the encrypted block, big endian (same as EncryptPacket).
// A little endian prefix is also accepted if it's the only plausible one.
func ReadFrame(reader *bufio.Reader) ([]byte, error) {
	header := make([]byte, 2)

	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}

	length := int(binary.BigEndian.Uint16(header))

	if !plausibleFrameLength(length) {
		le := int(binary.LittleEndian.Uint16(header))
		if !plausibleFrameLength(le) {
			return nil, fmt.Errorf("%w: invalid length header %x", ErrMalformed, header)
		}
		length = le
	}

	frame := make([]byte, 2+length)
	copy(frame, header)

	if _, err := io.ReadFull(reader, frame[2:]); err != nil {
		return nil, err
	}

	return frame, nil
}

func plausibleFrameLength(length int) bool {
	return length > 0 && length <= MaxFrameLength && length%8 == 0
}

// DecryptPacket decrypts a full frame (length prefix included).
func DecryptPacket(frame []byte) (NetworkMessage, error) {
	if len(frame) < 2+8 {
		return NetworkMessage{}, fmt.Errorf("%w: frame too short (%d)", ErrMalformed, len(frame))
	}

	// Decrypt the packet & rebuild it for request interpretation
	decrypt := security.DecryptXTEA(frame[2:])

	if len(decrypt) < 3 {
		return NetworkMessage{}, fmt.Errorf("%w: could not decrypt", ErrMalformed)
	}

	packetLength := int(binary.BigEndian.Uint16(decrypt[0:2]))

	if packetLength < 1 || 2+packetLength > len(decrypt) {
		return NetworkMessage{}, fmt.Errorf("%w: invalid inner length %d (have %d)", ErrMalformed, packetLength, len(decrypt)-2)
	}

	// Ok. We should now have the actual packet data.
	packetData := decrypt[2 : 2+packetLength]

	// The checksum isn't enforced since we can't confirm every client version sends it the same way
	if checksum := decrypt[2+packetLength:]; len(checksum) >= md5Length {
		if !bytes.Equal(checksum[:md5Length], security.EncryptMD5(packetData)) {
			log.Debug().Msg("Packet MD5 checksum mismatch")
		}
	}

	// Packet type
	packetType := PacketType(packetData[0])

	return NetworkMessage{packetType, packetData[1:]}, nil
}

func EncryptPacket(packetType PacketCode, packetData []byte, statusCode StatusCode) []byte {
	// Prepend packetType + statusCode before packetData
	packetData = append([]byte{
		packetType.Code(),
		statusCode.Code(),
		0x09,
	}, packetData...)

	length := len(packetData)
	lenBytes := make([]byte, 2)

	binary.BigEndian.PutUint16(lenBytes, uint16(length))

	md5 := security.EncryptMD5(packetData)

	input := append(append(lenBytes, packetData...), md5...)
	xtea := security.EncryptXTEA(input)

	length = len(xtea)
	outLen := make([]byte, 2)
	binary.BigEndian.PutUint16(outLen, uint16(length))

	output := append(outLen, xtea...)

	return output
}
