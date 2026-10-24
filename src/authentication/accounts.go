package authentication

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/iagoMAO/Botzin.OpenMASE/database"
	"github.com/iagoMAO/Botzin.OpenMASE/items"
	"github.com/iagoMAO/Botzin.OpenMASE/progression"
	"github.com/rs/zerolog/log"
)

var (
	ErrInvalidNick     = errors.New("nick deve ter de 3 a 16 caracteres (letras, números, _ - .)")
	ErrInvalidPassword = errors.New("senha deve ter de 4 a 32 caracteres")
	ErrNickTaken       = errors.New("nick já está em uso")
	ErrUserNotFound    = errors.New("usuário não encontrado")
)

var nickPattern = regexp.MustCompile(`^[A-Za-z0-9_\-.]{3,16}$`)

// HashMD5 returns the lowercase hex MD5, as the client sends username & password.
func HashMD5(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

type Account struct {
	Id      int
	Nick    string
	XP      int
	Level   int
	Patent  int
	Points  int
	Credits int
	Gold    int
	Ranking int
	Matches int
	Wins    int
	Kills   int
	Deaths  int
	Banned  bool
}

// FindAccount looks a user up by nick (case insensitive) or by numeric id.
func FindAccount(nickOrId string) (Account, error) {
	var a Account
	var banned int

	err := database.DB.QueryRow(`
		SELECT id, username, xp, level, patent, points, credits, gold, ranking, matches, wins, kills, deaths, banned
		FROM users WHERE username = ? COLLATE NOCASE OR CAST(id AS TEXT) = ?
		ORDER BY (username = ? COLLATE NOCASE) DESC LIMIT 1`, nickOrId, nickOrId, nickOrId).
		Scan(&a.Id, &a.Nick, &a.XP, &a.Level, &a.Patent, &a.Points, &a.Credits, &a.Gold, &a.Ranking, &a.Matches, &a.Wins, &a.Kills, &a.Deaths, &banned)

	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrUserNotFound
	}

	a.Banned = banned != 0

	return a, err
}

// Register creates an account with the starter items and the first bot equipped.
func Register(nick string, password string, startingCredits int, startingGold int) (int, error) {
	nick = strings.TrimSpace(nick)

	if !nickPattern.MatchString(nick) {
		return 0, ErrInvalidNick
	}

	if len(password) < 4 || len(password) > 32 {
		return 0, ErrInvalidPassword
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var existing int
	if err := tx.QueryRow("SELECT COUNT(*) FROM users WHERE username = ? COLLATE NOCASE OR username_md5 = ?", nick, HashMD5(nick)).Scan(&existing); err != nil {
		return 0, err
	}

	if existing > 0 {
		return 0, ErrNickTaken
	}

	starters, err := items.Starters(tx)
	if err != nil {
		return 0, err
	}

	// Equip the first bot and the first weapon that fits in it
	var bot *items.Item
	for i := range starters {
		if starters[i].Class == items.ClassBot {
			bot = &starters[i]
			break
		}
	}

	var st, dx, iq, ht int
	if bot != nil {
		st, dx, iq, ht = bot.ST, bot.DX, bot.IQ, bot.HT
	}

	res, err := tx.Exec(`
		INSERT INTO users (username, username_md5, password_md5, st, dx, iq, ht, credits, gold, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nick, HashMD5(nick), HashMD5(password), st, dx, iq, ht, startingCredits, startingGold, time.Now().Unix())
	if err != nil {
		return 0, err
	}

	id64, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	id := int(id64)

	weaponEquipped := false
	for _, item := range starters {
		equip := false

		if bot != nil && item.Id == bot.Id {
			equip = true
		} else if bot != nil && item.Class == items.ClassWeapon && !weaponEquipped && item.Payload <= bot.Payload {
			equip = true
			weaponEquipped = true
		}

		if err := items.Grant(tx, id, item, equip); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	if err := progression.RecalculateRanking(); err != nil {
		log.Warn().Msgf("Failed to recalculate ranking: %s", err)
	}

	log.Info().Msgf("Registered user %s (id %d) with %d starter items", nick, id, len(starters))

	return id, nil
}

func SetPassword(userId int, password string) error {
	if len(password) < 4 || len(password) > 32 {
		return ErrInvalidPassword
	}

	_, err := database.DB.Exec("UPDATE users SET password_md5 = ? WHERE id = ?", HashMD5(password), userId)
	return err
}

func SetBanned(userId int, banned bool) error {
	value := 0
	if banned {
		value = 1
	}

	_, err := database.DB.Exec("UPDATE users SET banned = ? WHERE id = ?", value, userId)
	return err
}

// AddCurrency adds (or removes, never below zero) credits or gold.
func AddCurrency(userId int, column string, amount int) (int, error) {
	if column != "credits" && column != "gold" {
		return 0, fmt.Errorf("invalid currency %s", column)
	}

	_, err := database.DB.Exec(fmt.Sprintf("UPDATE users SET %[1]s = MAX(0, %[1]s + ?) WHERE id = ?", column), amount, userId)
	if err != nil {
		return 0, err
	}

	var balance int
	err = database.DB.QueryRow(fmt.Sprintf("SELECT %s FROM users WHERE id = ?", column), userId).Scan(&balance)

	return balance, err
}
