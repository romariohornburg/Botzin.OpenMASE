package packets

import (
	"bytes"

	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
)

type LoginRequestPacket struct {
	EXECRC       string
	PasswordHash string
	UsernameHash string
	ClanTag      string
}

type LoginAnswerPacket struct {
	StatusCode  protocol.StatusCode
	MagicNumber []byte
	ClientGUID  []byte
}

type LoginErrorPacket struct {
	StatusCode protocol.StatusCode
}

// Compose sends the error the way the client expects it: exactly two fields,
// MASE_ERROR followed by the actual reason (MASE_ALREADY_LOGGED shares MASE_OK's value).
func (p LoginErrorPacket) Compose() []byte {
	data := []byte{
		uint8(p.StatusCode.Code()),
	}

	return protocol.EncryptPacket(protocol.LoginAnswer, data, protocol.MASE_ERROR)
}

func (p LoginAnswerPacket) Compose() []byte {
	var buf bytes.Buffer

	buf.Write(p.MagicNumber)
	buf.WriteByte(0x09)
	buf.Write(p.ClientGUID)

	return protocol.EncryptPacket(protocol.LoginAnswer, buf.Bytes(), protocol.MASE_OK)
}
