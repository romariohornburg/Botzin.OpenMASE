package packets

import (
	"bytes"
	"net"
	"strconv"

	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"
)

type LobbyLevel int

const (
	Novice   LobbyLevel = 1
	Advanced LobbyLevel = 2
	Pro      LobbyLevel = 3
	G2       LobbyLevel = 888
)

type Lobby struct {
	Level       LobbyLevel
	Address     string
	MaxPlayers  uint8
	Status      uint8
	BotCount    uint8
	PlayerCount uint8
	LastSeen    int64
}

// IPv4 returns the lobby address as 4 bytes + port. The protocol only supports IPv4.
func (l Lobby) IPv4() ([]byte, uint16, bool) {
	host, portStr, err := net.SplitHostPort(l.Address)
	if err != nil {
		return nil, 0, false
	}

	ip := net.ParseIP(host).To4()
	port, err := strconv.Atoi(portStr)

	if ip == nil || err != nil || port <= 0 || port > 65535 {
		return nil, 0, false
	}

	return ip, uint16(port), true
}

type GameMasterInfoRequest struct {
	QueryFlags uint8
	Session    uint32
	Key        uint32
}

func (p GameMasterInfoRequest) Compose() []byte {
	var buf bytes.Buffer

	buf.WriteByte(byte(protocol.GameMasterInfoRequest))
	buf.WriteByte(p.QueryFlags) // flags
	buf.Write(data.U32ToBytes(p.Session))
	buf.Write(data.U32ToBytes(p.Key))

	return buf.Bytes()
}

type GameMasterInfoResponse struct {
	QueryFlags   uint8
	Session      uint32
	Key          uint32
	GameType     string
	MissionType  string
	MaxPlayers   uint8
	RegionMask   uint32
	Version      uint32
	Status       uint8
	BotCount     uint8
	ProcessorMhz uint32
	PlayerCount  uint8
}

type MasterServerListRequest struct {
	QueryFlags  uint8
	Session     uint32
	Key         uint32
	GameType    string
	MissionType string
	MinPlayers  uint8
	MaxPlayers  uint8
	RegionMask  uint8
	Version     uint32
	FilterFlags uint8
	MaxBots     uint8
	MinCPU      uint16
	BuddyCount  uint8
}

type MasterServerListResponse struct {
	PacketIndex uint8
	PacketTotal uint8
	Session     uint32
	Key         uint32
	Lobbies     []Lobby
}

func (p MasterServerListResponse) Compose() []byte {
	var buf bytes.Buffer

	buf.WriteByte(byte(protocol.MasterServerListResponse))
	buf.WriteByte(0) // flags
	buf.Write(data.U32ToBytes(p.Session))
	buf.Write(data.U32ToBytes(p.Key))
	buf.WriteByte(p.PacketIndex)
	buf.WriteByte(p.PacketTotal)
	var addresses bytes.Buffer
	count := 0

	for _, lobby := range p.Lobbies {
		ip, port, ok := lobby.IPv4()
		if !ok {
			continue
		}

		addresses.Write(ip)
		addresses.Write(data.U16ToBytes(port))
		count++
	}

	buf.Write(data.U16ToBytes(uint16(count)))
	buf.Write(addresses.Bytes())

	if count <= 0 {
		buf.Write([]byte{0, 0, 0, 0})
		buf.Write(data.U16ToBytes(0))
	}

	return buf.Bytes()
}
