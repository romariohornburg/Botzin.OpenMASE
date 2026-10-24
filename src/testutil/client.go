// Package testutil builds client side packets for tests.
package testutil

import (
	"encoding/binary"

	"github.com/iagoMAO/Botzin.OpenMASE/security"
)

// ClientFrame builds a frame the way the game client sends it:
// length prefix + XTEA(inner length + type + payload + md5).
func ClientFrame(packetType byte, payload []byte) []byte {
	packetData := append([]byte{packetType}, payload...)

	inner := make([]byte, 2)
	binary.BigEndian.PutUint16(inner, uint16(len(packetData)))
	inner = append(inner, packetData...)
	inner = append(inner, security.EncryptMD5(packetData)...)

	encrypted := security.EncryptXTEA(inner)

	frame := make([]byte, 2)
	binary.BigEndian.PutUint16(frame, uint16(len(encrypted)))

	return append(frame, encrypted...)
}

// The client starts every request (except the logins) with the type and char($MASE_OK).
const maseOK = 102

func join(values [][]byte) []byte {
	var out []byte
	for i, value := range values {
		if i > 0 {
			out = append(out, 0x09)
		}
		out = append(out, value...)
	}
	return out
}

// Request mirrors char($MASE_OK) @ v1 TAB v2 ... (shop, attributes, avatar setup, avatar query).
func Request(values ...[]byte) []byte {
	return append([]byte{maseOK}, join(values)...)
}

// TabRequest mirrors char($MASE_OK) TAB v1 TAB v2 ... (score, payload check, buddy requests).
func TabRequest(values ...[]byte) []byte {
	return append([]byte{maseOK, 0x09}, join(values)...)
}

// BuddyLogin mirrors the buddy login: SCR_PackInt(magic) TAB SCR_PackInt(guid) TAB nick (no status byte).
func BuddyLogin(magic []byte, guid []byte, nick string) []byte {
	return join([][]byte{magic, guid, []byte(nick)})
}
