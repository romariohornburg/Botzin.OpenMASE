package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iagoMAO/Botzin.OpenMASE/authentication"
	"github.com/iagoMAO/Botzin.OpenMASE/core"
	"github.com/iagoMAO/Botzin.OpenMASE/database"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/testutil"
	"github.com/iagoMAO/Botzin.OpenMASE/utils"
	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"
)

type testClient struct {
	t      *testing.T
	conn   net.Conn
	reader *bufio.Reader
}

func setupServer(t *testing.T) (maseAddr string, buddyAddr string) {
	t.Helper()

	utils.SetConfig(utils.Config{
		XTEA_KEY:                   "ea818d6680d950355f25f0fef854373e",
		SESSION_TOKEN_AUTH:         true,
		STARTING_CREDITS:           100000,
		SCORE_KILL_POINTS:          10,
		SCORE_ALIVE_POINTS:         5,
		SCORE_WIN_POINTS:           50,
		SCORE_DROP_PENALTY:         20,
		MIN_SCORE_INTERVAL_SECONDS: 60,
		CREDITS_PER_POINT:          10,
		SELL_RATIO_PERCENT:         50,
		ATTRIB_MIN:                 0,
		ATTRIB_MAX:                 250,
		ATTRIB_POINTS_PER_LEVEL:    2,
	})
	cfg = utils.GetConfig()

	if err := database.Initialize(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.DB.Close() })

	if _, err := database.LoadItems("../sql/items.sql", false); err != nil {
		t.Fatal(err)
	}

	serve := func(handler func(net.Conn)) string {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { listener.Close() })

		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				go handler(conn)
			}
		}()

		return listener.Addr().String()
	}

	return serve(handleConnection), serve(handleBuddyConnection)
}

func dial(t *testing.T, addr string) *testClient {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	return &testClient{t: t, conn: conn, reader: bufio.NewReader(conn)}
}

func (c *testClient) send(packetType protocol.PacketType, payload []byte) {
	c.t.Helper()
	if _, err := c.conn.Write(testutil.ClientFrame(byte(packetType), payload)); err != nil {
		c.t.Fatal(err)
	}
}

// expect reads until a packet of the given type arrives and returns its status and body.
func (c *testClient) expect(packetType protocol.PacketType) (protocol.StatusCode, []byte) {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	for {
		frame, err := protocol.ReadFrame(c.reader)
		if err != nil {
			c.t.Fatalf("waiting for packet %d: %s", packetType, err)
		}

		message, err := protocol.DecryptPacket(frame)
		if err != nil {
			c.t.Fatal(err)
		}

		if message.Type.Code() == byte(packetType) {
			// status, 0x09, body
			body := []byte{}
			if len(message.Payload) > 2 {
				body = message.Payload[2:]
			}
			return protocol.StatusCode(message.Payload[0]), body
		}
	}
}

// expectClosed asserts the server dropped the connection.
func (c *testClient) expectClosed() {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	for {
		if _, err := protocol.ReadFrame(c.reader); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				c.t.Fatal("connection still open")
			}
			return
		}
	}
}

func loginPayload(nick string, password string) []byte {
	return []byte(strings.Repeat("0", 64) + authentication.HashMD5(password) + authentication.HashMD5(nick))
}

func login(t *testing.T, addr string, nick string, password string) (*testClient, int, int) {
	t.Helper()

	client := dial(t, addr)
	client.send(protocol.LoginRequest, loginPayload(nick, password))

	status, body := client.expect(protocol.LoginAnswer)
	if status != protocol.MASE_OK {
		t.Fatalf("login failed with status %d", status)
	}

	fields := bytes.Split(body, []byte{0x09})
	return client, data.SCR_StrToInt(fields[0]), data.SCR_StrToInt(fields[1])
}

func userValue(t *testing.T, id int, column string) int {
	t.Helper()
	var value int
	if err := database.DB.QueryRow("SELECT "+column+" FROM users WHERE id = ?", id).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestServerFlow(t *testing.T) {
	maseAddr, buddyAddr := setupServer(t)

	id, err := authentication.Register("Tester", "segredo", cfg.STARTING_CREDITS, 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := authentication.Register("tester", "outra", 0, 0); err != authentication.ErrNickTaken {
		t.Fatalf("expected nick taken, got %v", err)
	}

	friendId, _ := authentication.Register("Amigo", "segredo", 0, 0)

	t.Run("wrong password is refused and disconnected", func(t *testing.T) {
		client := dial(t, maseAddr)
		client.send(protocol.LoginRequest, loginPayload("Tester", "errada"))

		if status, _ := client.expect(protocol.LoginAnswer); status != protocol.MASE_INVALID_LOGINPASS {
			t.Fatalf("got status %d", status)
		}
		client.expectClosed()
	})

	t.Run("malformed login does not crash", func(t *testing.T) {
		client := dial(t, maseAddr)
		client.send(protocol.LoginRequest, []byte("short"))
		client.expectClosed()

		garbage := dial(t, maseAddr)
		garbage.conn.Write([]byte{0x00, 0x08, 1, 2, 3, 4, 5, 6, 7, 8, 0xFF, 0xFF, 0xFF})
		garbage.expectClosed()
	})

	client, token, guid := login(t, maseAddr, "Tester", "segredo")

	if guid != id || token == id || token == 0 {
		t.Fatalf("unexpected login answer: token %d guid %d", token, guid)
	}

	t.Run("starter items and equipped bot", func(t *testing.T) {
		client.send(protocol.UserBootRequest, []byte{0x09})
		client.expect(protocol.UserDataAnswer)
		client.expect(protocol.AvatarAttribLoadAnswer)
		client.expect(protocol.AvatarSetupLoadAnswer)
		client.expect(protocol.MaseShowGUIAnswer)
	})

	t.Run("duplicate login is refused", func(t *testing.T) {
		second := dial(t, maseAddr)
		second.send(protocol.LoginRequest, loginPayload("Tester", "segredo"))

		// The client wants MASE_ERROR plus the reason (MASE_ALREADY_LOGGED has MASE_OK's value)
		if status, body := second.expect(protocol.LoginAnswer); status != protocol.MASE_ERROR || protocol.StatusCode(body[0]) != protocol.MASE_ALREADY_LOGGED {
			t.Fatalf("got status %d %v", status, body)
		}
	})

	t.Run("buddy login requires the token", func(t *testing.T) {
		impostor := dial(t, buddyAddr)
		impostor.send(protocol.LoginRequest, testutil.BuddyLogin(data.SCR_PackInt(token+1), data.SCR_PackInt(id), "Tester"))

		if status, _ := impostor.expect(protocol.LoginAnswer); status != protocol.MASE_ERROR {
			t.Fatalf("impostor got status %d", status)
		}
		impostor.expectClosed()

		// Friend isn't logged in on MASE at all
		offline := dial(t, buddyAddr)
		offline.send(protocol.LoginRequest, testutil.BuddyLogin(data.SCR_PackInt(token), data.SCR_PackInt(friendId), "Amigo"))
		offline.expectClosed()

		buddyClient := dial(t, buddyAddr)
		buddyClient.send(protocol.LoginRequest, testutil.BuddyLogin(data.SCR_PackInt(token), data.SCR_PackInt(id), "Tester"))

		if status, _ := buddyClient.expect(protocol.LoginAnswer); status != protocol.MASE_OK {
			t.Fatalf("got status %d", status)
		}

		// Malformed buddy packets are ignored
		buddyClient.send(protocol.PrivateMessage, []byte{0x09})
		buddyClient.send(protocol.BuddyResponse, testutil.TabRequest(data.SCR_PackInt(friendId)))
		buddyClient.send(protocol.PublicMessage, testutil.TabRequest(data.SCR_PackInt(1), []byte("oi")))
		buddyClient.expect(protocol.PublicMessage)
	})

	t.Run("shop buy, sell and info", func(t *testing.T) {
		credits := userValue(t, id, "credits")

		// DualCannon costs 48000
		client.send(protocol.ShopBuyRequest, testutil.Request(data.SCR_PackInt(6)))
		_, body := client.expect(protocol.ShopBuyAnswer)
		if protocol.StatusCode(body[0]) != protocol.SHOP_BUY_DONE {
			t.Fatalf("buy status %d", body[0])
		}

		if got := userValue(t, id, "credits"); got != credits-48000 {
			t.Fatalf("credits %d, want %d", got, credits-48000)
		}

		client.send(protocol.ShopBuyRequest, testutil.Request(data.SCR_PackInt(6)))
		if _, body := client.expect(protocol.ShopBuyAnswer); protocol.StatusCode(body[0]) != protocol.SHOP_ALREADY_HAVE {
			t.Fatalf("second buy status %d", body[0])
		}

		// Starter items can't be sold
		client.send(protocol.ShopSellRequest, testutil.Request(data.SCR_PackInt(1)))
		if _, body := client.expect(protocol.ShopSellAnswer); protocol.StatusCode(body[0]) != protocol.SHOP_CANT_SELL {
			t.Fatalf("sell starter status %d", body[0])
		}

		client.send(protocol.ShopSellRequest, testutil.Request(data.SCR_PackInt(6)))
		if _, body := client.expect(protocol.ShopSellAnswer); protocol.StatusCode(body[0]) != protocol.SHOP_SELL_DONE {
			t.Fatalf("sell status %d", body[0])
		}

		if got := userValue(t, id, "credits"); got != credits-24000 {
			t.Fatalf("credits after sell %d, want %d", got, credits-24000)
		}

		// DualCannon: id, panel (class), credits, gold, stock left, mount point, payload
		client.send(protocol.ShopInfoRequest, testutil.Request(data.SCR_PackInt(6)))
		status, body := client.expect(protocol.ShopInfoAnswer)
		var info []int
		for _, field := range bytes.Split(body, []byte{0x09}) {
			info = append(info, data.SCR_StrToInt(field))
		}
		if want := []int{6, 1, 48000, 0, 10000, 3, 3}; status != protocol.MASE_OK || fmt.Sprint(info) != fmt.Sprint(want) {
			t.Fatalf("shop info status %d, got %v want %v", status, info, want)
		}
	})

	t.Run("shop gift", func(t *testing.T) {
		client.send(protocol.ShopGiftRequest, testutil.Request(data.SCR_PackInt(12), []byte("amigo")))
		if _, body := client.expect(protocol.ShopGiftAnswer); protocol.StatusCode(body[0]) != protocol.SHOP_BUY_DONE {
			t.Fatalf("gift status %d", body[0])
		}

		var count int
		database.DB.QueryRow("SELECT COUNT(*) FROM user_items WHERE user_id = ? AND item_id = 12", friendId).Scan(&count)
		if count != 1 {
			t.Fatal("friend didn't get the gift")
		}

		client.send(protocol.ShopGiftRequest, testutil.Request(data.SCR_PackInt(12), []byte("ninguem")))
		if _, body := client.expect(protocol.ShopGiftAnswer); protocol.StatusCode(body[0]) != protocol.SHOP_DONT_HAVE {
			t.Fatalf("gift to unknown status %d", body[0])
		}
	})

	t.Run("attributes are validated", func(t *testing.T) {
		attribs := func(st, dx, iq, ht int) []byte {
			return testutil.Request(data.SCR_PackInt(17), data.SCR_PackInt(st), data.SCR_PackInt(dx), data.SCR_PackInt(iq), data.SCR_PackInt(ht))
		}

		client.send(protocol.AvatarAttribSaveRequest, attribs(200, 200, 200, 200))
		if status, _ := client.expect(protocol.AvatarAttribSaveAnswer); status != protocol.MASE_ERROR {
			t.Fatalf("cheated attributes accepted (%d)", status)
		}

		client.send(protocol.AvatarAttribSaveRequest, attribs(150, 50, 100, 100))
		if status, _ := client.expect(protocol.AvatarAttribSaveAnswer); status != protocol.MASE_OK {
			t.Fatalf("valid attributes refused (%d)", status)
		}

		if userValue(t, id, "st") != 150 {
			t.Fatal("attributes not saved")
		}

		// Short request
		client.send(protocol.AvatarAttribSaveRequest, testutil.Request(data.SCR_PackInt(17)))
		client.expect(protocol.AvatarAttribSaveAnswer)
	})

	t.Run("equipment is validated", func(t *testing.T) {
		setup := func(ids ...int) []byte {
			var fields [][]byte
			for _, id := range ids {
				fields = append(fields, data.SCR_PackInt(id))
			}
			return testutil.Request(fields...)
		}

		// Not owned
		client.send(protocol.AvatarSetupSaveRequest, setup(17, 26))
		if status, _ := client.expect(protocol.AvatarSetupSaveAnswer); status != protocol.MASE_ERROR {
			t.Fatalf("unowned item accepted (%d)", status)
		}

		client.send(protocol.AvatarSetupSaveRequest, setup(17, 1, 2))
		if status, _ := client.expect(protocol.AvatarSetupSaveAnswer); status != protocol.MASE_OK {
			t.Fatalf("valid setup refused (%d)", status)
		}

		client.send(protocol.AvatarCheckPayLoadRequest, []byte{0x09})
		status, body := client.expect(protocol.AvatarCheckPayLoadAnswer)
		fields := bytes.Split(body, []byte{0x09})
		if status != protocol.MASE_OK || data.SCR_StrToInt(fields[0]) != 10 || data.SCR_StrToInt(fields[1]) != 2 {
			t.Fatalf("payload check: status %d fields %v", status, fields)
		}
	})

	t.Run("match score from the host", func(t *testing.T) {
		packed := func(values ...int) [][]byte {
			var out [][]byte
			for _, v := range values {
				out = append(out, data.SCR_PackInt(v))
			}
			return out
		}

		// host, mission, turn time, wind, clients per team, bots per client; then guid, kills, bots alive, dropped
		matchReport := func(host int, clients ...[]int) []byte {
			fields := packed(host, 1, 100, 0, 1, 3)
			for _, c := range clients {
				fields = append(fields, packed(c...)...)
			}
			return testutil.TabRequest(fields...)
		}

		rejected := func(name string, payload []byte) {
			client.send(protocol.ScoreRequest, payload)
			if status, _ := client.expect(protocol.ScoreAnswer); status != protocol.MASE_ERROR {
				t.Fatalf("%s: accepted (%d)", name, status)
			}
		}

		rejected("malformed", testutil.TabRequest(data.SCR_PackInt(id)))
		rejected("other host", matchReport(friendId, []int{friendId, 3, 3, 0}, []int{id, 0, 0, 0}))
		// Amigo isn't logged in, so this report can't be real
		rejected("offline opponent", matchReport(id, []int{id, 3, 3, 0}, []int{friendId, 0, 0, 0}))

		friend, _, _ := login(t, maseAddr, "Amigo", "segredo")
		defer friend.conn.Close()

		client.send(protocol.ScoreRequest, matchReport(id, []int{id, 2, 1, 0}, []int{friendId, 0, 0, 0}))

		status, body := client.expect(protocol.ScoreAnswer)
		if status != protocol.MASE_OK {
			t.Fatalf("score status %d", status)
		}

		// count, then guid, sign, score, sign, credits per player (what gameOnMASEScoreAnswer reads)
		fields := bytes.Split(body, []byte{0x09})
		got := fmt.Sprintf("%d %d %s %d %s %d %d %s %d %s %d",
			data.SCR_StrToInt(fields[0]),
			data.SCR_StrToInt(fields[1]), fields[2], data.SCR_StrToInt(fields[3]), fields[4], data.SCR_StrToInt(fields[5]),
			data.SCR_StrToInt(fields[6]), fields[7], data.SCR_StrToInt(fields[8]), fields[9], data.SCR_StrToInt(fields[10]))
		// winner: 2 kills * 10 + 1 alive * 5 + 50 = 75 points, 750 credits
		if want := fmt.Sprintf("2 %d + 75 + 750 %d + 0 + 0", id, friendId); got != want {
			t.Fatalf("score answer %q, want %q", got, want)
		}

		if xp := userValue(t, id, "xp"); xp != 75 {
			t.Fatalf("xp %d", xp)
		}
		if wins := userValue(t, id, "wins"); wins != 1 {
			t.Fatalf("wins %d", wins)
		}
		if matches := userValue(t, friendId, "matches"); matches != 1 {
			t.Fatalf("friend matches %d", matches)
		}
		if rank := userValue(t, id, "ranking"); rank != 1 {
			t.Fatalf("ranking %d", rank)
		}
		if total := userValue(t, id, "totalRK"); total != 2 {
			t.Fatalf("totalRK %d", total)
		}

		// The opponent gets its data refreshed
		friend.expect(protocol.UserDataAnswer)

		// Too fast: ignored
		rejected("flood", matchReport(id, []int{id, 2, 1, 0}, []int{friendId, 0, 0, 0}))
		if xp := userValue(t, id, "xp"); xp != 75 {
			t.Fatalf("xp changed to %d", xp)
		}
	})

	t.Run("keep-alive and disconnect", func(t *testing.T) {
		// Keep-alives need no answer
		client.send(protocol.KeepAliveSignal, nil)

		client.send(protocol.DisconnectRequest, []byte("OFF"))
		client.expect(protocol.DisconnectAnswer)
		client.expectClosed()

		// Session is released, so logging in again works
		deadline := time.Now().Add(2 * time.Second)
		for core.IsLoggedIn(id) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		login(t, maseAddr, "Tester", "segredo")
	})

	t.Run("banned account", func(t *testing.T) {
		var out bytes.Buffer
		runCommand(&out, "ban(Amigo)")

		banned := dial(t, maseAddr)
		banned.send(protocol.LoginRequest, loginPayload("Amigo", "segredo"))
		if status, body := banned.expect(protocol.LoginAnswer); status != protocol.MASE_ERROR || protocol.StatusCode(body[0]) != protocol.MASE_ACCOUNT_BANNED {
			t.Fatalf("got status %d %v", status, body)
		}
	})
}

func TestConsoleCommands(t *testing.T) {
	setupServer(t)

	run := func(line string) string {
		var out bytes.Buffer
		runCommand(&out, line)
		return out.String()
	}

	cases := []struct {
		line string
		want string
	}{
		{"register(Fulano, senha123)", "criada"},
		{`register("Fulano", "x")`, "Erro"},
		{"credits(fulano, 500)", "100500"},
		{"credits fulano -999999", "0 de credits"},
		{"gold(Fulano, 10)", "10 de gold"},
		{"xp(Fulano, 5500)", "nível 10"},
		{"give(Fulano, 26)", "NovaGun"},
		{"give(Fulano, 26)", "já tem"},
		{"info(Fulano)", "NovaGun"},
		{"ranking()", "Fulano"},
		{"setpass(Fulano, nova123)", "alterada"},
		{"ban(Fulano)", "banido=true"},
		{"unban(Fulano)", "banido=false"},
		{"kick(Fulano)", "offline"},
		{"broadcast(Olá a todos, FF0000)", "0 jogador"},
		{"lobbies()", "lobby"},
		{"items()", "BABYBOY"},
		{"reloaditems()", "itens carregados"},
		{"help", "register"},
		{"naoexiste()", "desconhecido"},
		{"credits(Fulano)", "Uso"},
		{"info(Ninguem)", "não encontrado"},
		{"()", "desconhecido"},
		{"", ""},
	}

	for _, c := range cases {
		if got := run(c.line); !strings.Contains(got, c.want) {
			t.Errorf("%q: got %q, want it to contain %q", c.line, got, c.want)
		}
	}
}

func TestRegisterHTTP(t *testing.T) {
	setupServer(t)

	server := newRegisterHandler()

	post := func(form string) (int, string) {
		req := httptest.NewRequest("POST", "/register", strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	if code, body := post("nick=Novato&password=abcd&confirm=abcd"); code != 200 || !strings.Contains(body, "criada") {
		t.Fatalf("register failed: %d %s", code, body)
	}

	if code, _ := post("nick=Novato&password=abcd&confirm=abcd"); code != 400 {
		t.Fatalf("duplicate accepted: %d", code)
	}

	if code, _ := post("nick=Outro&password=abcd&confirm=dcba"); code != 400 {
		t.Fatalf("mismatched passwords accepted: %d", code)
	}

	if code, body := post("nick=<script>&password=abcd&confirm=abcd"); code != 400 || strings.Contains(body, "<script>") {
		t.Fatalf("bad nick accepted or not escaped: %d", code)
	}

	// Rate limit kicks in after a few attempts from the same IP
	for i := 0; i < registerLimit; i++ {
		post("nick=X&password=abcd&confirm=abcd")
	}
	if code, _ := post("nick=Ultimo&password=abcd&confirm=abcd"); code != 429 {
		t.Fatalf("rate limit not applied: %d", code)
	}
}
