package shop

import (
	"database/sql"
	"errors"
	"strconv"

	"github.com/rs/zerolog/log"

	"github.com/iagoMAO/Botzin.OpenMASE/database"
	"github.com/iagoMAO/Botzin.OpenMASE/items"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol/packets"
)

func countOwned(q items.Querier, userId int, itemId int) (int, error) {
	var count int
	err := q.QueryRow("SELECT COUNT(*) FROM user_items WHERE item_id = ? AND user_id = ?", itemId, userId).Scan(&count)
	return count, err
}

// pay charges the item price from the payer inside the transaction.
func pay(tx *sql.Tx, userId int, item items.Item) (protocol.StatusCode, bool) {
	var userCredits, userGold int

	if err := tx.QueryRow("SELECT credits, gold FROM users WHERE id = ?", userId).Scan(&userCredits, &userGold); err != nil {
		return protocol.MASE_ERROR, false
	}

	if userCredits < item.Credits {
		return protocol.SHOP_NO_CREDITS, false
	}

	if userGold < item.Gold {
		return protocol.SHOP_NO_GOLD, false
	}

	if _, err := tx.Exec("UPDATE users SET credits = credits - ?, gold = gold - ? WHERE id = ?", item.Credits, item.Gold, userId); err != nil {
		log.Error().Msgf("Error charging user %d: %s", userId, err)
		return protocol.MASE_ERROR, false
	}

	return protocol.SHOP_BUY_DONE, true
}

func BuyItem(userId int, itemId int) packets.ShopBuyAnswerPacket {
	answer := func(status protocol.StatusCode) packets.ShopBuyAnswerPacket {
		return packets.ShopBuyAnswerPacket{ShopBuyAnswerType: status, Item: packets.AvatarItemData{Id: itemId, UserId: userId}}
	}

	item, err := items.Get(itemId)
	if err != nil {
		log.Debug().Msgf("User %d tried to buy unknown item %d", userId, itemId)
		return answer(protocol.SHOP_NO_STOCK)
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return answer(protocol.MASE_ERROR)
	}
	defer tx.Rollback()

	if count, err := countOwned(tx, userId, itemId); err != nil || count != 0 {
		return answer(protocol.SHOP_ALREADY_HAVE)
	}

	if !inStock(tx, item) {
		return answer(protocol.SHOP_NO_STOCK)
	}

	if status, ok := pay(tx, userId, item); !ok {
		return answer(status)
	}

	if err := items.Grant(tx, userId, item, false); err != nil {
		log.Error().Msgf("Error granting item %d to user %d: %s", itemId, userId, err)
		return answer(protocol.MASE_ERROR)
	}

	if err := tx.Commit(); err != nil {
		return answer(protocol.MASE_ERROR)
	}

	log.Debug().Msgf("User %d bought item %d", userId, itemId)

	return answer(protocol.SHOP_BUY_DONE)
}

// SellItem refunds SELL_RATIO_PERCENT of the item price. Equipped and unsellable items are refused.
func SellItem(userId int, itemId int, ratioPercent int) packets.ShopSellAnswerPacket {
	answer := func(status protocol.StatusCode) packets.ShopSellAnswerPacket {
		return packets.ShopSellAnswerPacket{Status: status, ItemId: itemId}
	}

	item, err := items.Get(itemId)
	if err != nil {
		return answer(protocol.SHOP_DONT_HAVE)
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return answer(protocol.MASE_ERROR)
	}
	defer tx.Rollback()

	var userItemId, enabled int
	err = tx.QueryRow("SELECT id, enabled FROM user_items WHERE user_id = ? AND item_id = ? LIMIT 1", userId, itemId).Scan(&userItemId, &enabled)
	if err != nil {
		return answer(protocol.SHOP_DONT_HAVE)
	}

	if !item.CanSell || enabled != 0 {
		return answer(protocol.SHOP_CANT_SELL)
	}

	refundCredits := item.Credits * ratioPercent / 100
	refundGold := item.Gold * ratioPercent / 100

	if _, err := tx.Exec("DELETE FROM user_items WHERE id = ?", userItemId); err != nil {
		return answer(protocol.MASE_ERROR)
	}

	if _, err := tx.Exec("UPDATE users SET credits = credits + ?, gold = gold + ? WHERE id = ?", refundCredits, refundGold, userId); err != nil {
		return answer(protocol.MASE_ERROR)
	}

	if err := tx.Commit(); err != nil {
		return answer(protocol.MASE_ERROR)
	}

	log.Debug().Msgf("User %d sold item %d for %d credits / %d gold", userId, itemId, refundCredits, refundGold)

	return answer(protocol.SHOP_SELL_DONE)
}

var ErrGiftRecipient = errors.New("recipient not found")

// FindRecipient resolves a gift recipient sent either as a nick or as a packed GUID.
func FindRecipient(nick string, guid int) (int, string, error) {
	var id int
	var name string

	err := database.DB.QueryRow("SELECT id, username FROM users WHERE username = ? COLLATE NOCASE", nick).Scan(&id, &name)
	if err == nil {
		return id, name, nil
	}

	if guid > 0 {
		if err := database.DB.QueryRow("SELECT id, username FROM users WHERE id = ?", guid).Scan(&id, &name); err == nil {
			return id, name, nil
		}
	}

	return 0, "", ErrGiftRecipient
}

// GiftItem buys the item with the sender's money and gives it to the recipient.
func GiftItem(userId int, recipientId int, itemId int) packets.ShopGiftAnswerPacket {
	answer := func(status protocol.StatusCode) packets.ShopGiftAnswerPacket {
		return packets.ShopGiftAnswerPacket{Status: status, ItemId: itemId}
	}

	if recipientId == 0 || recipientId == userId {
		return answer(protocol.SHOP_DONT_HAVE)
	}

	item, err := items.Get(itemId)
	if err != nil {
		return answer(protocol.SHOP_NO_STOCK)
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return answer(protocol.MASE_ERROR)
	}
	defer tx.Rollback()

	if count, err := countOwned(tx, recipientId, itemId); err != nil || count != 0 {
		return answer(protocol.SHOP_ALREADY_HAVE)
	}

	if !inStock(tx, item) {
		return answer(protocol.SHOP_NO_STOCK)
	}

	if status, ok := pay(tx, userId, item); !ok {
		return answer(status)
	}

	if err := items.Grant(tx, recipientId, item, false); err != nil {
		return answer(protocol.MASE_ERROR)
	}

	if err := tx.Commit(); err != nil {
		return answer(protocol.MASE_ERROR)
	}

	log.Info().Msgf("User %d gifted item %d to user %d", userId, itemId, recipientId)

	return answer(protocol.SHOP_BUY_DONE)
}

// Info returns the current price and stock of an item.
func Info(itemId int) packets.ShopInfoAnswerPacket {
	item, err := items.Get(itemId)
	if err != nil {
		return packets.ShopInfoAnswerPacket{Status: protocol.MASE_ERROR, ItemId: itemId}
	}

	owners, err := items.Owners(database.DB, itemId)
	if err != nil {
		return packets.ShopInfoAnswerPacket{Status: protocol.MASE_ERROR, ItemId: itemId}
	}

	return packets.ShopInfoAnswerPacket{
		Status:    protocol.MASE_OK,
		ItemId:    item.Id,
		Class:     item.Class,
		Credits:   item.Credits,
		Gold:      item.Gold,
		Available: item.Available(owners),
		Mount:     item.Mount,
		Payload:   item.Payload,
	}
}

// inStock checks the per world limit of the item.
func inStock(q items.Querier, item items.Item) bool {
	owners, err := items.Owners(q, item.Id)
	return err == nil && item.Available(owners) > 0
}

// ItemName is used for gift notifications.
func ItemName(itemId int) string {
	item, err := items.Get(itemId)
	if err != nil || item.Name == "" {
		return "#" + strconv.Itoa(itemId)
	}
	return item.Name
}
