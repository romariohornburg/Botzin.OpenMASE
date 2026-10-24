package packets

import (
	"bytes"

	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"
)

type ShopBuyAnswerPacket struct {
	ShopBuyAnswerType protocol.StatusCode
	Item              AvatarItemData
}

// shopAnswer is the layout shared by the buy/sell/gift answers: result code, then the item id.
func shopAnswer(packetType protocol.PacketType, status protocol.StatusCode, itemId int) []byte {
	var buf bytes.Buffer

	buf.WriteByte(status.Code())

	buf.WriteByte(0x09)

	buf.Write(data.SCR_PackInt(itemId))

	return protocol.EncryptPacket(packetType, buf.Bytes(), protocol.MASE_OK)
}

func (p ShopBuyAnswerPacket) Compose() []byte {
	return shopAnswer(protocol.ShopBuyAnswer, p.ShopBuyAnswerType, p.Item.Id)
}

type ShopSellAnswerPacket struct {
	Status protocol.StatusCode
	ItemId int
}

func (p ShopSellAnswerPacket) Compose() []byte {
	return shopAnswer(protocol.ShopSellAnswer, p.Status, p.ItemId)
}

type ShopGiftAnswerPacket struct {
	Status protocol.StatusCode
	ItemId int
}

func (p ShopGiftAnswerPacket) Compose() []byte {
	return shopAnswer(protocol.ShopGiftAnswer, p.Status, p.ItemId)
}

// ShopInfoAnswerPacket refreshes one item of the client's shop (the catalog itself is built into
// the client). Fields: item id, shop panel (BuyItens_FrameC<class>), credits, gold, available
// stock, mount point and payload.
type ShopInfoAnswerPacket struct {
	Status    protocol.StatusCode
	ItemId    int
	Class     int
	Credits   int
	Gold      int
	Available int
	Mount     int
	Payload   int
}

func (p ShopInfoAnswerPacket) Compose() []byte {
	var buf bytes.Buffer

	for i, value := range []int{p.ItemId, p.Class, p.Credits, p.Gold, p.Available, p.Mount, p.Payload} {
		if i > 0 {
			buf.WriteByte(0x09)
		}
		buf.Write(data.SCR_PackInt(value))
	}

	return protocol.EncryptPacket(protocol.ShopInfoAnswer, buf.Bytes(), p.Status)
}
