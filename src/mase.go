package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/iagoMAO/Botzin.OpenMASE/authentication"
	"github.com/iagoMAO/Botzin.OpenMASE/avatar"
	"github.com/iagoMAO/Botzin.OpenMASE/buddy"
	"github.com/iagoMAO/Botzin.OpenMASE/core"
	"github.com/iagoMAO/Botzin.OpenMASE/progression"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol/packets"
	"github.com/iagoMAO/Botzin.OpenMASE/shop"
	"github.com/iagoMAO/Botzin.OpenMASE/utils"
	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"

	"github.com/rs/zerolog/log"
)

func handleConnection(conn net.Conn) {
	// Close once we're done, again
	defer onMASEDisconnect(conn)
	defer conn.Close()
	defer utils.Recover("MASE connection")

	readMessages(conn, "MASE", func(message protocol.NetworkMessage) bool {
		if message.Type == protocol.LoginRequest {
			return handleLogin(conn, message)
		}

		session := core.GetSession(conn)

		// Everything else requires an authenticated session
		if session == nil {
			log.Debug().Msgf("Received packet %d without a session from %s", message.Type.Code(), conn.RemoteAddr())
			return true
		}

		return handleMASEPacket(session, message)
	})
}

// onMASEDisconnect drops the session and lets the contacts know the user went offline.
func onMASEDisconnect(conn net.Conn) {
	session, offline := core.RemoveSession(conn)

	if session == nil {
		return
	}

	// Without the game connection the buddy connection isn't authenticated anymore
	if !offline {
		session.Close()
		return
	}

	refreshContactsOf(session.UserId)
}

func handleLogin(conn net.Conn, message protocol.NetworkMessage) bool {
	request, ok := authentication.ParseLoginRequest(message.Payload)

	if !ok {
		log.Warn().Msgf("Malformed login request from %s (%d bytes)", conn.RemoteAddr(), len(message.Payload))
		conn.Write(packets.LoginErrorPacket{StatusCode: protocol.MASE_ERROR}.Compose())
		return false
	}

	token := 0
	if cfg.SESSION_TOKEN_AUTH {
		token = core.NewToken()
	}

	// Handle Login request
	id, login := authentication.Login(request, token)

	conn.Write(login.Compose())

	if id == 0 {
		return false
	}

	core.RegisterConnection(conn, id, core.ConnTypeMASE)
	core.SetToken(id, token)

	log.Info().Msgf("User %d logged in from %s", id, conn.RemoteAddr())

	return true
}

func handleMASEPacket(session *core.Session, message protocol.NetworkMessage) bool {
	switch message.Type {
	case protocol.UserBootRequest:
		session.SetStatus(protocol.BUDDY_STATUS_ONLINE)

		user := authentication.GetUserInfoPacket(session.UserId)
		attribs := avatar.GetAvatarAttrib(session.UserId)
		avatarInfo := avatar.GetAvatarInfo(session.UserId)
		guiPacket := packets.MaseShowGUIAnswerPacket{StatusCode: protocol.MASE_OK}
		broadcastPacket := packets.BroadcastAnswerPacket{StatusCode: protocol.MASE_OK, MessageColor: 102, MessageText: cfg.WELCOME_BROADCAST_MESSAGE}

		session.WriteMASE(user.Compose(), attribs.Compose(), avatarInfo.Compose(), guiPacket.Compose(), broadcastPacket.Compose())

		refreshContactsOf(session.UserId)

		log.Debug().Msgf("Received User Boot Request: %s from User %d", hex.EncodeToString(message.Payload), session.UserId)
	case protocol.UserDataRequest:
		sendUserData(session)

		log.Debug().Msgf("Received User Data Request: %s from User %d", hex.EncodeToString(message.Payload), session.UserId)
	case protocol.AvatarAttribLoadRequest:
		session.WriteMASE(avatar.GetAvatarAttrib(session.UserId).Compose())
	case protocol.AvatarSetupLoadRequest:
		session.WriteMASE(avatar.GetAvatarInfo(session.UserId).Compose())
	case protocol.ShopBuyRequest:
		itemId := payloadInt(message)

		log.Debug().Msgf("Received Shop Buy Request for item %d from User Id %d", itemId, session.UserId)

		if itemId <= 0 {
			return true
		}

		session.WriteMASE(shop.BuyItem(session.UserId, itemId).Compose())
		sendUserData(session)
	case protocol.ShopSellRequest:
		itemId := payloadInt(message)

		log.Info().Msgf("Received Shop Sell Request for item %d from User Id %d: %s", itemId, session.UserId, hex.EncodeToString(message.Payload))

		if itemId <= 0 {
			return true
		}

		session.WriteMASE(shop.SellItem(session.UserId, itemId, cfg.SELL_RATIO_PERCENT).Compose())
		sendUserData(session)
	case protocol.ShopGiftRequest:
		handleShopGift(session, message)
	case protocol.ShopInfoRequest:
		log.Debug().Msgf("Received Shop Info Request from User Id %d: %s", session.UserId, hex.EncodeToString(message.Payload))

		itemId := payloadInt(message)
		if itemId <= 0 {
			return true
		}

		session.WriteMASE(shop.Info(itemId).Compose())
	case protocol.ServerQueryAvatarRequest:
		userId := payloadInt(message)

		if userId <= 0 {
			return true
		}

		// Querying the avatars of the other players means we're joining a match
		if session.GetStatus() != protocol.BUDDY_STATUS_INGAME {
			session.SetStatus(protocol.BUDDY_STATUS_INGAME)
			refreshContactsOf(session.UserId)
		}

		session.WriteMASE(avatar.GetAvatarSetupData(userId).Compose())
	case protocol.AvatarSetupSaveRequest:
		itemIds := fieldInts(message)

		log.Debug().Msgf("Received Avatar Setup Save Request from User Id %d %v", session.UserId, itemIds)

		request := packets.AvatarSetupSaveRequestPacket{
			ItemIds: itemIds,
		}

		session.WriteMASE(avatar.SaveAvatarSetup(session.UserId, request).Compose())
	case protocol.AvatarAttribSaveRequest:
		if len(message.Fields()) < 5 {
			session.WriteMASE(packets.AvatarAttribSaveAnswerPacket{Status: protocol.MASE_ERROR}.Compose())
			return true
		}

		request := packets.AvatarAttribSaveRequestPacket{
			BotId: message.IntField(0),
			ST:    message.IntField(1),
			DX:    message.IntField(2),
			IQ:    message.IntField(3),
			HT:    message.IntField(4),
		}

		session.WriteMASE(avatar.SaveAvatarAttrib(session.UserId, request).Compose())
	case protocol.AvatarCheckPayLoadRequest:
		session.WriteMASE(avatar.CheckPayload(session.UserId, fieldInts(message)).Compose())
	case protocol.ScoreRequest:
		handleScore(session, message)
	case protocol.BroadcastRequest:
		session.WriteMASE(packets.BroadcastAnswerPacket{StatusCode: protocol.MASE_OK, MessageColor: 102, MessageText: cfg.WELCOME_BROADCAST_MESSAGE}.Compose())
	case protocol.KeepAliveSignal, protocol.PingAnswer:
		// Keep-alives and answers to server pings need no reply
	case protocol.HackBusterRequest:
		session.WriteMASE(packets.EmptyAnswer{Type: protocol.HackBusterAnswer, StatusCode: protocol.MASE_OK}.Compose())
	case protocol.DisconnectRequest:
		session.WriteMASE(packets.EmptyAnswer{Type: protocol.DisconnectAnswer, StatusCode: protocol.MASE_OK}.Compose())
		return false
	default:
		log.Debug().Msgf("Received unhandled packet %d from User %d:\n%s", message.Type.Code(), session.UserId, hex.Dump(message.Payload))
	}

	return true
}

// payloadInt reads requests that carry a single packed integer right after the first byte.
func payloadInt(message protocol.NetworkMessage) int {
	if len(message.Payload) < 2 {
		return 0
	}
	return data.SCR_StrToInt(message.Payload[1:])
}

// fieldInts reads a tab separated list of packed integers, ignoring empty/zero entries.
func fieldInts(message protocol.NetworkMessage) []int {
	var out []int
	for _, raw := range message.Fields() {
		if value := data.SCR_StrToInt(raw); value > 0 {
			out = append(out, value)
		}
	}
	return out
}

func sendUserData(session *core.Session) {
	session.WriteMASE(authentication.GetUserInfoPacket(session.UserId).Compose())
}

func handleShopGift(session *core.Session, message protocol.NetworkMessage) {
	// Layout not confirmed against the client yet: item id, then the recipient (nick or packed GUID)
	log.Info().Msgf("Received Shop Gift Request from User Id %d: %s", session.UserId, hex.EncodeToString(message.Payload))

	itemId := message.IntField(0)
	recipientRaw := message.Field(1)

	recipientId, recipientNick, err := shop.FindRecipient(string(recipientRaw), data.SCR_StrToInt(recipientRaw))
	if err != nil || itemId <= 0 {
		session.WriteMASE(packets.ShopGiftAnswerPacket{Status: protocol.SHOP_DONT_HAVE, ItemId: itemId}.Compose())
		return
	}

	answer := shop.GiftItem(session.UserId, recipientId, itemId)
	session.WriteMASE(answer.Compose())
	sendUserData(session)

	if answer.Status != protocol.SHOP_BUY_DONE {
		return
	}

	sender := authentication.GetUserInfo(session.UserId)
	buddy.SendMessage(recipientId, fmt.Sprintf("%s te deu de presente: %s!", sender.Nick, shop.ItemName(itemId)), "00FF00")

	if recipient := core.GetSessionByUserId(recipientId); recipient != nil {
		recipient.WriteMASE(avatar.GetAvatarInfo(recipientId).Compose())
	}

	log.Info().Msgf("User %d gifted %d to %s", session.UserId, itemId, recipientNick)
}

// handleScore processes the end of match report sent by the match host (see progression.ScoreReport).
func handleScore(session *core.Session, message protocol.NetworkMessage) {
	log.Debug().Msgf("Received Score Request from User %d: %s", session.UserId, hex.EncodeToString(message.Payload))

	fail := func(reason string, args ...any) {
		log.Warn().Msgf("Rejected score from user %d: "+reason, append([]any{session.UserId}, args...)...)
		session.WriteMASE(packets.ScoreAnswer{StatusCode: protocol.MASE_ERROR}.Compose())
	}

	interval := time.Duration(cfg.MIN_SCORE_INTERVAL_SECONDS) * time.Second
	if !session.LastScoreAt.IsZero() && time.Since(session.LastScoreAt) < interval {
		fail("sent too fast")
		return
	}

	report, err := progression.ParseScoreReport(message.Fields())
	if err != nil {
		fail("%s", err)
		return
	}

	// Only the host reports, and only for itself as the host
	if report.HostGUID != session.UserId {
		fail("host guid %d doesn't match the session", report.HostGUID)
		return
	}

	// Everybody who finished the match must be logged in, otherwise the report is made up
	for _, entry := range report.Entries {
		if !entry.Dropped && !core.IsLoggedIn(entry.GUID) {
			fail("player %d isn't logged in", entry.GUID)
			return
		}
	}

	rules := progression.Rules{
		KillPoints:      cfg.SCORE_KILL_POINTS,
		AlivePoints:     cfg.SCORE_ALIVE_POINTS,
		WinPoints:       cfg.SCORE_WIN_POINTS,
		DropPenalty:     cfg.SCORE_DROP_PENALTY,
		CreditsPerPoint: cfg.CREDITS_PER_POINT,
	}

	results, err := progression.ApplyReport(report, rules)
	if err != nil {
		log.Error().Msgf("Failed to apply score from user %d: %s", session.UserId, err)
		session.WriteMASE(packets.ScoreAnswer{StatusCode: protocol.MASE_ERROR}.Compose())
		return
	}

	session.LastScoreAt = time.Now()

	answer := packets.ScoreAnswer{StatusCode: protocol.MASE_OK}

	for _, result := range results {
		answer.Entries = append(answer.Entries, packets.ScoreAnswerEntry{
			GUID:    result.Entry.GUID,
			Score:   result.Rewards.Score,
			Credits: result.Rewards.Credits,
		})

		log.Info().Msgf("Match (mission %d, host %d): user %d %+v -> %+d score, +%d XP, +%d credits (level %d, rank %d/%d)",
			report.MissionIndex, report.HostGUID, result.Entry.GUID, result.Entry, result.Rewards.Score,
			result.Rewards.XP, result.Rewards.Credits, result.Level, result.Ranking, result.TotalRK)

		notifyMatchResult(result)
	}

	session.WriteMASE(answer.Compose())
}

// notifyMatchResult refreshes the player's data and tells them what they got.
func notifyMatchResult(result progression.Result) {
	player := core.GetSessionByUserId(result.Entry.GUID)
	if player == nil || !result.Known {
		return
	}

	// Back from the match
	if player.GetStatus() == protocol.BUDDY_STATUS_INGAME {
		player.SetStatus(protocol.BUDDY_STATUS_ONLINE)
		refreshContactsOf(player.UserId)
	}

	player.WriteMASE(
		authentication.GetUserInfoPacket(player.UserId).Compose(),
		avatar.GetAvatarAttrib(player.UserId).Compose(),
	)

	if !cfg.SCORE_NOTIFY {
		return
	}

	text := fmt.Sprintf("%+d pontos, +%d XP, +%d créditos. Ranking: %dº de %d",
		result.Rewards.Score, result.Rewards.XP, result.Rewards.Credits, result.Ranking, result.TotalRK)
	if result.LevelUp() {
		text += fmt.Sprintf("<br>Subiu para o nível %d!", result.Level)
	}

	buddy.SendMessage(player.UserId, text, "E3FF0E")
}
