package buddy

import (
	"fmt"

	"github.com/iagoMAO/Botzin.OpenMASE/core"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol/packets"
)

func BroadcastPacket(message string, color string) []byte {
	if color != "" {
		message = fmt.Sprintf("<color:%s>%s", color, message)
	}

	packet := packets.BroadcastAnswerPacket{StatusCode: protocol.MASE_OK, MessageColor: 102, MessageText: message}
	return packet.Compose()
}

// BroadcastMessage sends an announcement to everyone connected. Returns how many got it.
func BroadcastMessage(message string, color string) int {
	packet := BroadcastPacket(message, color)
	sessions := core.GetAllSessions()

	for _, session := range sessions {
		session.WriteMASE(packet)
	}

	return len(sessions)
}

// SendMessage sends an announcement to a single user, if online.
func SendMessage(userId int, message string, color string) bool {
	session := core.GetSessionByUserId(userId)
	if session == nil {
		return false
	}

	session.WriteMASE(BroadcastPacket(message, color))
	return true
}
