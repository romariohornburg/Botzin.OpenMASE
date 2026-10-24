package avatar

import (
	"database/sql"

	"github.com/iagoMAO/Botzin.OpenMASE/authentication"
	"github.com/iagoMAO/Botzin.OpenMASE/database"
	"github.com/iagoMAO/Botzin.OpenMASE/items"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol/packets"
	"github.com/iagoMAO/Botzin.OpenMASE/utils"
	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"
	"github.com/rs/zerolog/log"
)

type AvatarAttrib struct {
	XP int
	ST int
	DX int
	IQ int
	HT int
}

func GetAvatarSetupData(id int) packets.ServerQueryAvatarAnswerPacket {
	user := authentication.GetUserInfo(id)

	rows, err := database.DB.Query(`
		SELECT 
			item.id,
			userItem.user_id,
			item.class,
			item.st,
			item.dx,
			item.iq,
			item.ht,
			item.payload,
			item.the_gen,
			userItem.enabled
		FROM user_items AS userItem
		JOIN items AS item 
			ON userItem.item_id = item.id
		WHERE userItem.user_id = ?;
	`, id)

	if err != nil {
		log.Error().Msgf("error: %s", err)
		return packets.ServerQueryAvatarAnswerPacket{}
	}

	defer rows.Close()

	var items []packets.AvatarItemData

	for rows.Next() {
		var item packets.AvatarItemData
		err := rows.Scan(
			&item.Id,
			&item.UserId,
			&item.Class,
			&item.ST,
			&item.DX,
			&item.IQ,
			&item.HT,
			&item.Payload,
			&item.TheGen,
			&item.Enabled,
		)

		if err != nil {
			log.Error().Msgf("error: %s", err)
			return packets.ServerQueryAvatarAnswerPacket{}
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		log.Error().Msgf("error: %s", err)
		return packets.ServerQueryAvatarAnswerPacket{}
	}

	var patent, ranking int
	database.DB.QueryRow("SELECT patent, ranking FROM users WHERE id = ?", id).Scan(&patent, &ranking)

	packet := packets.ServerQueryAvatarAnswerPacket{
		ClientGUID:       id,
		TotalAvatarItems: len(items),
		Items:            items,
		Nick:             user.Nick,
		ST:               user.ST,
		DX:               user.DX,
		IQ:               user.IQ,
		HT:               user.HT,
		XP:               user.XP,
		Patent:           patent,
		Ranking:          ranking,
	}

	return packet
}

func GetAvatarInfo(id int) packets.AvatarSetupLoadAnswerPacket {
	rows, err := database.DB.Query(`
		SELECT 
			item.id,
			userItem.user_id,
			item.class,
			userItem.st,
			userItem.dx,
			userItem.iq,
			userItem.ht,
			item.payload,
			item.the_gen,
			userItem.enabled
		FROM user_items AS userItem
		JOIN items AS item 
			ON userItem.item_id = item.id
		WHERE userItem.user_id = ?;
	`, id)

	if err != nil {
		log.Error().Msgf("error: %s", err)
		return packets.AvatarSetupLoadAnswerPacket{}
	}

	defer rows.Close()

	var items []packets.AvatarItemData

	for rows.Next() {
		var item packets.AvatarItemData
		err := rows.Scan(
			&item.Id,
			&item.UserId,
			&item.Class,
			&item.ST,
			&item.DX,
			&item.IQ,
			&item.HT,
			&item.Payload,
			&item.TheGen,
			&item.Enabled,
		)

		if err != nil {
			log.Error().Msgf("error: %s", err)
			return packets.AvatarSetupLoadAnswerPacket{}
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		log.Error().Msgf("error: %s", err)
		return packets.AvatarSetupLoadAnswerPacket{}
	}

	packet := packets.AvatarSetupLoadAnswerPacket{
		TotalAvatarItems: len(items),
		Items:            items,
	}

	return packet
}

func SaveAvatarSetup(userId int, request packets.AvatarSetupSaveRequestPacket) packets.AvatarSetupSaveAnswerPacket {
	owned, _, err := items.Owned(database.DB, userId)
	if err != nil {
		log.Error().Msgf("Failed to load inventory for user %d: %s", userId, err)
		return packets.AvatarSetupSaveAnswerPacket{Status: protocol.MASE_ERROR}
	}

	if _, err := items.ValidateSetup(owned, request.ItemIds); err != nil {
		log.Warn().Msgf("User %d sent an invalid avatar setup %v: %s", userId, request.ItemIds, err)
		return packets.AvatarSetupSaveAnswerPacket{Status: protocol.MASE_ERROR}
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return packets.AvatarSetupSaveAnswerPacket{Status: protocol.MASE_ERROR}
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE user_items SET enabled = 0 WHERE user_id = ?", userId)
	if err != nil {
		log.Error().Msgf("Failed to unequip items for user %d", userId)
		return packets.AvatarSetupSaveAnswerPacket{
			Status: protocol.MASE_ERROR,
		}
	}

	for _, item := range request.ItemIds {
		_, err = tx.Exec("UPDATE user_items SET enabled = 1 WHERE user_id = ? AND item_id = ?", userId, item)
		if err != nil {
			log.Error().Msgf("Failed to equip item %d for user %d", item, userId)
			return packets.AvatarSetupSaveAnswerPacket{
				Status: protocol.MASE_ERROR,
			}
		}

	}

	if err := tx.Commit(); err != nil {
		return packets.AvatarSetupSaveAnswerPacket{Status: protocol.MASE_ERROR}
	}

	return packets.AvatarSetupSaveAnswerPacket{
		Status: protocol.MASE_OK,
	}
}

func SaveAvatarAttrib(userId int, request packets.AvatarAttribSaveRequestPacket) packets.AvatarAttribSaveAnswerPacket {
	row := database.DB.QueryRow("SELECT id FROM user_items WHERE user_id = ? AND item_id = ?", userId, request.BotId)

	var id int

	err := row.Scan(&id)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Warn().Msgf("User %d tried to update unowned bot %d", userId, request.BotId)
		} else {
			log.Error().Msgf("Database error: %s", err)
		}
		return packets.AvatarAttribSaveAnswerPacket{
			Status: protocol.MASE_ERROR,
		}
	}

	bot, err := items.Get(request.BotId)
	if err != nil {
		return packets.AvatarAttribSaveAnswerPacket{Status: protocol.MASE_ERROR}
	}

	var level int
	database.DB.QueryRow("SELECT level FROM users WHERE id = ?", userId).Scan(&level)

	cfg := utils.GetConfig()
	limits := items.AttribLimits{Min: cfg.ATTRIB_MIN, Max: cfg.ATTRIB_MAX, PointsPerLevel: cfg.ATTRIB_POINTS_PER_LEVEL}

	if err := items.ValidateAttribs(bot, level, request.ST, request.DX, request.IQ, request.HT, limits); err != nil {
		log.Warn().Msgf("User %d sent invalid attributes for bot %d (%d/%d/%d/%d): %s", userId, request.BotId, request.ST, request.DX, request.IQ, request.HT, err)
		return packets.AvatarAttribSaveAnswerPacket{Status: protocol.MASE_ERROR}
	}

	query := "UPDATE user_items SET st = ?, dx = ?, iq = ?, ht = ? WHERE user_id = ? AND id = ?"

	_, err = database.DB.Exec(query, request.ST, request.DX, request.IQ, request.HT, userId, id)

	if err != nil {
		log.Debug().Msgf("Error: %s", err.Error())
		return packets.AvatarAttribSaveAnswerPacket{
			Status: protocol.MASE_ERROR,
		}
	}

	query = "UPDATE users SET st = ?, dx = ?, iq = ?, ht = ? WHERE id = ?"

	_, err = database.DB.Exec(query, request.ST, request.DX, request.IQ, request.HT, userId)

	if err != nil {
		log.Debug().Msgf("Error: %s", err.Error())
		return packets.AvatarAttribSaveAnswerPacket{
			Status: protocol.MASE_ERROR,
		}
	}

	return packets.AvatarAttribSaveAnswerPacket{
		Status: protocol.MASE_OK,
	}
}

func GetAvatarAttrib(id int) packets.AvatarAttribLoadAnswerPacket {
	row := database.DB.QueryRow("SELECT xp, st, dx, iq, ht FROM users WHERE id = ?", id)
	var u AvatarAttrib
	err := row.Scan(
		&u.XP,
		&u.ST,
		&u.DX,
		&u.IQ,
		&u.HT,
	)

	if err != nil {
		log.Error().Msgf("Error: %s", err)
		return packets.AvatarAttribLoadAnswerPacket{}
	}

	return packets.AvatarAttribLoadAnswerPacket{
		XP: string(data.SCR_PackInt(u.XP)),
		ST: string(data.SCR_PackInt(u.ST)),
		DX: string(data.SCR_PackInt(u.DX)),
		IQ: string(data.SCR_PackInt(u.IQ)),
		HT: string(data.SCR_PackInt(u.HT)),
	}
}

// CheckPayload verifies the given setup (or the equipped one, when empty) fits in the bot.
func CheckPayload(userId int, itemIds []int) packets.AvatarCheckPayLoadAnswerPacket {
	owned, equipped, err := items.Owned(database.DB, userId)
	if err != nil {
		return packets.AvatarCheckPayLoadAnswerPacket{Status: protocol.MASE_ERROR}
	}

	if len(itemIds) == 0 {
		for id, enabled := range equipped {
			if enabled {
				itemIds = append(itemIds, id)
			}
		}
	}

	var setup []items.Item
	for _, id := range itemIds {
		if item, ok := owned[id]; ok {
			setup = append(setup, item)
		}
	}

	capacity, used := items.PayloadUsage(setup)

	status := protocol.MASE_OK
	if _, err := items.ValidateSetup(owned, itemIds); err != nil {
		status = protocol.MASE_ERROR
	}

	return packets.AvatarCheckPayLoadAnswerPacket{Status: status, Capacity: capacity, Used: used}
}
