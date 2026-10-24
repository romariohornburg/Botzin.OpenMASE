package authentication

import (
	"strings"

	"github.com/iagoMAO/Botzin.OpenMASE/core"
	"github.com/iagoMAO/Botzin.OpenMASE/database"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol/packets"
	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"
)

// ParseLoginRequest splits the fixed width login payload.
func ParseLoginRequest(payload []byte) (packets.LoginRequestPacket, bool) {
	if len(payload) < 128 {
		return packets.LoginRequestPacket{}, false
	}

	payloadStr := string(payload)

	return packets.LoginRequestPacket{
		EXECRC:       payloadStr[0:64],
		PasswordHash: payloadStr[64:96],
		UsernameHash: payloadStr[96:128],
		ClanTag:      payloadStr[128:],
	}, true
}

// We receive a login request packet & return the answer packet.
// magicNumber is handed to the client and later used to authenticate the Buddy connection
// (0 = use the user id, the old behaviour).
func Login(packet packets.LoginRequestPacket, magicNumber int) (int, protocol.Packet) {
	var id int
	var password string
	var banned int

	err := database.DB.QueryRow("SELECT id, password_md5, banned FROM users WHERE username_md5 = ? COLLATE NOCASE", packet.UsernameHash).Scan(&id, &password, &banned)

	// Invalid password or inexistent user (the client doesn't really handle both seperately)
	if err != nil || password == "" || !strings.EqualFold(password, packet.PasswordHash) {
		return 0, packets.LoginErrorPacket{
			StatusCode: protocol.MASE_INVALID_LOGINPASS,
		}
	}

	if banned != 0 {
		return 0, packets.LoginErrorPacket{
			StatusCode: protocol.MASE_ACCOUNT_BANNED,
		}
	}

	if core.IsLoggedIn(id) {
		return 0, packets.LoginErrorPacket{
			StatusCode: protocol.MASE_ALREADY_LOGGED,
		}
	}

	if magicNumber == 0 {
		magicNumber = id
	}

	// We can login!
	return id, packets.LoginAnswerPacket{
		StatusCode:  protocol.MASE_OK,
		MagicNumber: data.SCR_PackInt(magicNumber),
		ClientGUID:  data.SCR_PackInt(id),
	}
}
