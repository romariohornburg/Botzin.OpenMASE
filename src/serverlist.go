package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol/packets"
	"github.com/iagoMAO/Botzin.OpenMASE/utils"
	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"

	"github.com/rs/zerolog/log"
)

// Lobbies that didn't send a heartbeat for this long are dropped
const lobbyTimeout = 3 * time.Minute

var (
	Lobbies      []packets.Lobby
	lobbiesMutex sync.Mutex
)

// GetLobbies returns a copy of the active lobbies.
func GetLobbies() []packets.Lobby {
	lobbiesMutex.Lock()
	defer lobbiesMutex.Unlock()

	return append([]packets.Lobby(nil), Lobbies...)
}

func CloseServerConnection(listener net.PacketConn) {
	// Before actually closing the connection, we need to remove this lobby (if it exists)
	lobbiesMutex.Lock()

	var activeLobbies []packets.Lobby

	for i := range Lobbies {
		if Lobbies[i].Address != listener.LocalAddr().String() {
			activeLobbies = append(activeLobbies, Lobbies[i])
		}
	}

	Lobbies = activeLobbies
	lobbiesMutex.Unlock()

	listener.Close()
}

func StartServerList() {
	defer utils.Recover("SERVERLIST listener")

	// Create the listener using ListenPacket for UDP
	listener, err := net.ListenPacket("udp", fmt.Sprintf("0.0.0.0:%s", cfg.SERVERLIST_PORT))

	if err != nil {
		log.Error().Msgf("Listening error: %s", err)
		return
	}

	log.Info().Msgf("SERVERLIST - Successfully started listening on port %s.", cfg.SERVERLIST_PORT)

	// Close the socket once we're done
	defer CloseServerConnection(listener)

	buffer := make([]byte, 2048)

	for {
		n, addr, err := listener.ReadFrom(buffer)

		if err != nil {
			log.Error().Msgf("Error thrown whilst reading packet: %s", err)
			continue
		}

		if n < 1 {
			continue
		}

		handleServerListPacket(listener, addr, buffer[:n])
	}
}

func handleServerListPacket(listener net.PacketConn, addr net.Addr, packet []byte) {
	// A malformed datagram must never take the server down
	defer utils.Recover("SERVERLIST packet")

	switch protocol.PacketType(packet[0]) {
	default:
		log.Debug().Msgf("packet type %d", packet[0])
	case protocol.GameMasterInfoResponse:
		handleGameInfoResponse(addr, packet)
	case protocol.GameHeartbeat:
		log.Debug().Msg("server heartbeat")

		lobbiesMutex.Lock()
		for i := range Lobbies {
			if Lobbies[i].Address == addr.String() {
				Lobbies[i].LastSeen = time.Now().Unix()
				break
			}
		}
		lobbiesMutex.Unlock()

		response := packets.GameMasterInfoRequest{QueryFlags: uint8(10), Session: uint32(0), Key: uint32(0)}

		writeDatagram(listener, addr, response.Compose())
	case protocol.MasterServerListRequest:
		handleServerListRequest(listener, addr, packet)
	}
}

func handleGameInfoResponse(addr net.Addr, packet []byte) {
	reader := data.NewReader(packet)
	reader.Skip(1)

	response := packets.GameMasterInfoResponse{
		QueryFlags:   reader.U8(),
		Session:      reader.U32(),
		Key:          reader.U32(),
		GameType:     reader.String(),
		MissionType:  reader.String(),
		MaxPlayers:   reader.U8(),
		RegionMask:   reader.U32(),
		Version:      reader.U32(),
		Status:       reader.U8(),
		BotCount:     reader.U8(),
		ProcessorMhz: reader.U32(),
		PlayerCount:  reader.U8(),
	}

	if reader.Err() != nil {
		log.Warn().Msgf("Malformed game info response from %s: %s", addr, hex.EncodeToString(packet))
		return
	}

	missionType, err := strconv.Atoi(response.MissionType)

	if err != nil {
		return
	}

	lobbiesMutex.Lock()
	defer lobbiesMutex.Unlock()

	for i := range Lobbies {
		if Lobbies[i].Address == addr.String() {
			Lobbies[i].MaxPlayers = response.MaxPlayers
			Lobbies[i].Status = response.Status
			Lobbies[i].BotCount = response.BotCount
			Lobbies[i].PlayerCount = response.PlayerCount
			Lobbies[i].LastSeen = time.Now().Unix()
			return
		}
	}

	Lobbies = append(Lobbies, packets.Lobby{
		Level:       packets.LobbyLevel(missionType),
		Address:     addr.String(),
		MaxPlayers:  response.MaxPlayers,
		Status:      response.Status,
		BotCount:    response.BotCount,
		PlayerCount: response.PlayerCount,
		LastSeen:    time.Now().Unix(),
	})

	log.Debug().Msgf("%#v", response)
}

func handleServerListRequest(listener net.PacketConn, addr net.Addr, packet []byte) {
	reader := data.NewReader(packet)
	reader.Skip(1)

	req := packets.MasterServerListRequest{}

	req.QueryFlags = reader.U8()
	req.Session = reader.U32()
	req.Key = reader.U32()

	// random 0xFF byte here
	reader.Skip(1)

	req.GameType = reader.String()
	req.MissionType = reader.String()

	if reader.Err() != nil {
		log.Warn().Msgf("Malformed server list request from %s: %s", addr, hex.EncodeToString(packet))
		return
	}

	log.Debug().Msgf("queryFlags: %d, session: %d, gameType: %s, missionType: %s", req.QueryFlags, req.Session, req.GameType, req.MissionType)

	lobbyLevel, err := strconv.Atoi(req.MissionType)
	if err != nil {
		log.Debug().Msgf("error: %s", err)
		return
	}

	var filteredLobbies []packets.Lobby
	var activeLobbies []packets.Lobby
	now := time.Now()

	lobbiesMutex.Lock()
	for _, lobby := range Lobbies {
		if now.Sub(time.Unix(lobby.LastSeen, 0)) <= lobbyTimeout {
			activeLobbies = append(activeLobbies, lobby)

			if int(lobby.Level) == lobbyLevel {
				filteredLobbies = append(filteredLobbies, lobby)
			}
		}
	}
	Lobbies = activeLobbies
	lobbiesMutex.Unlock()

	response := packets.MasterServerListResponse{
		PacketIndex: 0,
		PacketTotal: 1,
		Session:     req.Session,
		Key:         req.Key,
		Lobbies:     filteredLobbies,
	}

	writeDatagram(listener, addr, response.Compose())
}

func writeDatagram(listener net.PacketConn, addr net.Addr, packet []byte) {
	if _, err := listener.WriteTo(packet, addr); err != nil {
		log.Error().Msgf("Failed to send response to %s: %s", addr.String(), err)
	} else {
		log.Debug().Msgf("Successfully sent %d bytes to %s - \n%s", len(packet), addr.String(), hex.Dump(packet))
	}
}
