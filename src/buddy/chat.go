package buddy

import (
	"sync"

	"github.com/iagoMAO/Botzin.OpenMASE/protocol/packets"
)

// Messages kept per room so players joining can see the recent history
const roomHistorySize = 32

var (
	roomHistory      = make(map[int][]packets.PublicMessagePacket)
	roomHistoryMutex sync.Mutex
)

// AddRoomMessage stores a message in the room history, dropping the oldest when full.
func AddRoomMessage(roomId int, packet packets.PublicMessagePacket) {
	roomHistoryMutex.Lock()
	defer roomHistoryMutex.Unlock()

	history := append(roomHistory[roomId], packet)
	if len(history) > roomHistorySize {
		history = history[len(history)-roomHistorySize:]
	}

	roomHistory[roomId] = history
}

// RoomHistory returns a copy of the room history.
func RoomHistory(roomId int) []packets.PublicMessagePacket {
	roomHistoryMutex.Lock()
	defer roomHistoryMutex.Unlock()

	return append([]packets.PublicMessagePacket(nil), roomHistory[roomId]...)
}
