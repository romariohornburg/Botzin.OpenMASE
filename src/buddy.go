package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net"

	"github.com/iagoMAO/Botzin.OpenMASE/authentication"
	"github.com/iagoMAO/Botzin.OpenMASE/buddy"
	"github.com/iagoMAO/Botzin.OpenMASE/core"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol/packets"
	"github.com/iagoMAO/Botzin.OpenMASE/utils"
	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"

	"github.com/rs/zerolog/log"
)

// Longest chat message relayed (the client limits it too, this is just a safety net)
const maxChatMessageLength = 512

func BootBuddyRequest(session *core.Session) {
	if !session.HasBuddy() {
		return
	}

	buddies := buddy.GetCategorizedContacts(session.UserId)

	onlinePacket := packets.BootBuddyAnswerPacket{
		Status:              protocol.StatusCode(protocol.BUDDY_STATUS_ONLINE),
		TotalContactsOnList: len(buddies.Online),
		Contacts:            buddies.Online,
	}

	ingamePacket := packets.BootBuddyAnswerPacket{
		Status:              protocol.StatusCode(protocol.BUDDY_STATUS_INGAME),
		TotalContactsOnList: len(buddies.Ingame),
		Contacts:            buddies.Ingame,
	}

	offlinePacket := packets.BootBuddyAnswerPacket{
		Status:              protocol.StatusCode(protocol.BUDDY_STATUS_OFFLINE),
		TotalContactsOnList: len(buddies.Offline),
		Contacts:            buddies.Offline,
	}

	endPacket := packets.BootBuddyAnswerPacket{
		Status:              protocol.StatusCode(protocol.BUDDY_ENDOF_LIST),
		TotalContactsOnList: 0,
	}

	guiPacket := packets.MaseShowGUIAnswerPacket{StatusCode: protocol.MASE_OK}

	session.WriteBuddy(onlinePacket.Compose(), ingamePacket.Compose(), offlinePacket.Compose(), endPacket.Compose(), guiPacket.Compose())
}

// refreshContactsOf pushes an updated buddy list to every online contact of the user.
func refreshContactsOf(userId int) {
	for _, contact := range buddy.GetContacts(userId) {
		if contactSession := core.GetSessionByUserId(contact.GUID); contactSession != nil {
			BootBuddyRequest(contactSession)
		}
	}
}

func StartBuddyList() {
	defer utils.Recover("BUDDY listener")

	// Create the listener
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.BUDDY_PORT))

	if err != nil {
		log.Error().Msgf("Listening error: %s", err)
		return
	}

	log.Info().Msgf("BUDDY - Successfully started listening on port %s.", cfg.BUDDY_PORT)

	// Close the socket once we're done
	defer listener.Close()

	for {
		conn, err := listener.Accept()

		if err != nil {
			log.Error().Msgf("Error thrown whilst accepting connection: %s", err)
			continue
		}

		go handleBuddyConnection(conn)
	}
}

func handleBuddyConnection(conn net.Conn) {
	// Close once we're done, again
	defer core.RemoveSession(conn)
	defer conn.Close()
	defer utils.Recover("BUDDY connection")

	readMessages(conn, "BUDDY", func(message protocol.NetworkMessage) bool {
		if message.Type == protocol.LoginRequest {
			return handleBuddyLogin(conn, message)
		}

		session := core.GetSession(conn)

		if session == nil {
			return false
		}

		handleBuddyPacket(conn, session, message)
		return true
	})
}

// authenticateBuddy makes sure the buddy login belongs to a user that is logged in on MASE.
// The client sends SCR_PackInt($MASE_THEMAGICNUMBER) \t SCR_PackInt($LocalClientGUID) \t nick
// right after the packet type (no status byte), so the magic number is the first raw field.
// Either that magic number must match the token handed out on the MASE login, or (with
// SESSION_TOKEN_AUTH=false) the connection must come from the same IP as the MASE one.
func authenticateBuddy(conn net.Conn, message protocol.NetworkMessage) (int, bool) {
	fields := bytes.Split(message.Payload, []byte{'\t'})
	if len(fields) < 2 {
		return 0, false
	}

	magic := data.SCR_StrToInt(fields[0])
	userId := data.SCR_StrToInt(fields[1])

	if userId <= 0 {
		return 0, false
	}

	session := core.GetSessionByUserId(userId)
	if session == nil {
		return 0, false
	}

	maseAddr := session.MASEAddr()
	if maseAddr == nil {
		return 0, false
	}

	if !cfg.SESSION_TOKEN_AUTH {
		return userId, sameHost(conn.RemoteAddr(), maseAddr)
	}

	token := session.GetToken()

	return userId, token != 0 && magic == token
}

func sameHost(a net.Addr, b net.Addr) bool {
	hostA, _, errA := net.SplitHostPort(a.String())
	hostB, _, errB := net.SplitHostPort(b.String())

	return errA == nil && errB == nil && hostA == hostB
}

func handleBuddyLogin(conn net.Conn, message protocol.NetworkMessage) bool {
	log.Debug().Msgf("Received Buddy Login Request: %s", hex.EncodeToString(message.Payload))

	userId, ok := authenticateBuddy(conn, message)

	if !ok {
		log.Warn().Msgf("Rejected buddy login from %s (fields %q)", conn.RemoteAddr(), message.Fields())
		conn.Write(protocol.EncryptPacket(protocol.LoginAnswer, []byte{}, protocol.MASE_ERROR))
		return false
	}

	conn.Write(protocol.EncryptPacket(protocol.LoginAnswer, []byte{}, protocol.MASE_OK))
	core.RegisterConnection(conn, userId, core.ConnTypeBUDDY)

	return true
}

func handleBuddyPacket(conn net.Conn, session *core.Session, message protocol.NetworkMessage) {
	switch message.Type {
	case protocol.AddContactRequest:
		log.Debug().Msgf("Received Add Contact Request: %s", hex.EncodeToString(message.Payload))

		userId := message.IntField(1)

		if userId <= 0 || userId == session.UserId {
			return
		}

		if err := buddy.AddContact(session.UserId, userId); err != nil {
			return
		}

		contacts := buddy.GetContacts(session.UserId)

		response := packets.AddContactAnswerPacket{
			Status:              protocol.MASE_OK,
			TotalContactsOnList: len(contacts),
			Contacts:            contacts,
		}

		if buddySession := core.GetSessionByUserId(userId); buddySession != nil {
			pending := buddy.GetOutgoingRequests(userId, protocol.BUDDY_ANSWER_REQUEST)

			pendingPacket := packets.BootStatusAnswerPacket{
				Status:              protocol.StatusCode(protocol.BUDDY_ANSWER_REQUEST),
				TotalContactsOnList: len(pending),
				Contacts:            pending,
			}

			buddySession.WriteBuddy(pendingPacket.Compose())
		}

		session.WriteBuddy(response.Compose())
	case protocol.FindContactRequest:
		request := packets.FindContactRequestPacket{
			GUID: message.IntField(1),
			Name: string(message.Field(2)),
		}

		if len(request.Name) < 1 {
			return
		}

		contacts := buddy.QueryContacts(request.Name)

		response := packets.FindContactAnswerPacket{
			Status:              protocol.MASE_OK,
			TotalContactsOnList: len(contacts),
			Contacts:            contacts,
		}

		log.Debug().Msgf("Received Find Contact Request: %#v", request)

		session.WriteBuddy(response.Compose())
	case protocol.BootBuddyRequest:
		log.Debug().Msgf("Received Boot Buddy Request: %s", hex.EncodeToString(message.Payload))

		BootBuddyRequest(session)
		refreshContactsOf(session.UserId)
	case protocol.PrivateMessage:
		buddyId := message.IntField(1)
		text := string(message.Field(2))

		if buddyId <= 0 || len(text) == 0 || len(text) > maxChatMessageLength {
			return
		}

		buddySession := core.GetSessionByUserId(buddyId)

		if buddySession == nil {
			return
		}

		user := authentication.GetUserInfo(session.UserId)

		packet := packets.PrivateMessagePacket{
			Status:  protocol.MASE_OK,
			Contact: packets.BuddyContactInfo{GUID: session.UserId, Name: user.Nick},
			Message: text,
		}

		buddySession.WriteBuddy(packet.Compose())
	case protocol.BuddyResponse:
		log.Debug().Msgf("Received Buddy Response: %s", hex.EncodeToString(message.Payload))

		buddyId := message.IntField(1)
		rawStatus := message.Field(2)

		if buddyId <= 0 || len(rawStatus) < 1 {
			return
		}

		status := protocol.BuddyStatus(rawStatus[0])

		switch status {
		case protocol.BUDDY_ANSWER_ACCEPTED, protocol.BUDDY_ANSWER_REJECTED, protocol.BUDDY_ANSWER_REMOVED:
		default:
			return
		}

		buddy.RespondContact(session.UserId, buddyId, status)

		if buddySession := core.GetSessionByUserId(buddyId); buddySession != nil {
			contacts := buddy.GetOutgoingRequests(buddyId, status)
			contactsPacket := packets.BootStatusAnswerPacket{
				Status:              protocol.StatusCode(status),
				TotalContactsOnList: len(contacts),
				Contacts:            contacts,
			}

			buddySession.WriteBuddy(contactsPacket.Compose(), protocol.EncryptPacket(protocol.BootStatusAnswer, []byte{}, protocol.StatusCode(protocol.BUDDY_ENDOF_LIST)))
		}
	case protocol.BootStatusRequest:
		log.Debug().Msgf("Received Boot Status: %s", hex.EncodeToString(message.Payload))

		pending := buddy.GetIncomingRequests(session.UserId)
		rejected := buddy.GetOutgoingRequests(session.UserId, protocol.BUDDY_ANSWER_REJECTED)
		accepted := buddy.GetOutgoingRequests(session.UserId, protocol.BUDDY_ANSWER_ACCEPTED)
		removed := buddy.GetOutgoingRequests(session.UserId, protocol.BUDDY_ANSWER_REMOVED)

		lists := []struct {
			status   protocol.BuddyStatus
			contacts []packets.BuddyContactInfo
		}{
			{protocol.BUDDY_ANSWER_REQUEST, pending},
			{protocol.BUDDY_ANSWER_REJECTED, rejected},
			{protocol.BUDDY_ANSWER_ACCEPTED, accepted},
			{protocol.BUDDY_ANSWER_REMOVED, removed},
		}

		var out [][]byte
		for _, list := range lists {
			out = append(out, packets.BootStatusAnswerPacket{
				Status:              protocol.StatusCode(list.status),
				TotalContactsOnList: len(list.contacts),
				Contacts:            list.contacts,
			}.Compose())
		}
		out = append(out, protocol.EncryptPacket(protocol.BootStatusAnswer, []byte{}, protocol.StatusCode(protocol.BUDDY_ENDOF_LIST)))

		session.WriteBuddy(out...)
	case protocol.PublicMessage:
		roomId := message.IntField(1)
		text := string(message.Field(2))

		// An empty message means the player just joined the room and wants the history
		if len(text) <= 0 {
			for _, packet := range buddy.RoomHistory(roomId) {
				session.WriteBuddy(packet.Compose())
			}
			return
		}

		if len(text) > maxChatMessageLength {
			return
		}

		user := authentication.GetUserInfo(session.UserId)

		packet := packets.PublicMessagePacket{
			Status:  protocol.MASE_OK,
			RoomId:  roomId,
			Nick:    user.Nick,
			Message: text,
		}

		buddy.AddRoomMessage(roomId, packet)

		composed := packet.Compose()
		for _, other := range core.GetAllSessions() {
			other.WriteBuddy(composed)
		}
	case protocol.KeepAliveSignal:
	default:
		log.Debug().Msgf("Received unhandled buddy packet %d:\n%s", message.Type.Code(), hex.Dump(message.Payload))
	}
}
