package packets

import (
	"bytes"

	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"
)

type ScoreAnswerEntry struct {
	GUID    int
	Score   int
	Credits int
}

// ScoreAnswer goes back to the match host, which shows it on the end game screen
// (gameOnMASEScoreAnswer): the number of entries, then per player its GUID, the score
// and the credits, each value preceded by its sign ("+" or "-") in a separate field.
type ScoreAnswer struct {
	StatusCode protocol.StatusCode
	Entries    []ScoreAnswerEntry
}

func writeSigned(buf *bytes.Buffer, value int) {
	sign := "+"
	if value < 0 {
		sign = "-"
		value = -value
	}

	buf.WriteByte(0x09)
	buf.WriteString(sign)
	buf.WriteByte(0x09)
	buf.Write(data.SCR_PackInt(value))
}

func (p ScoreAnswer) Compose() []byte {
	var buf bytes.Buffer

	buf.Write(data.SCR_PackInt(len(p.Entries)))

	for _, entry := range p.Entries {
		buf.WriteByte(0x09)
		buf.Write(data.SCR_PackInt(entry.GUID))
		writeSigned(&buf, entry.Score)
		writeSigned(&buf, entry.Credits)
	}

	return protocol.EncryptPacket(protocol.ScoreAnswer, buf.Bytes(), p.StatusCode)
}

// EmptyAnswer is a status only answer (ping, disconnect, hackbuster...).
type EmptyAnswer struct {
	Type       protocol.PacketType
	StatusCode protocol.StatusCode
}

func (p EmptyAnswer) Compose() []byte {
	return protocol.EncryptPacket(p.Type, []byte{}, p.StatusCode)
}
