package items

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/iagoMAO/Botzin.OpenMASE/database"
)

// Item classes as used by the client
const (
	ClassWeapon = 1
	ClassBot    = 2
	ClassTool   = 4
	ClassDrone  = 5
)

type Item struct {
	Id      int
	Class   int
	Name    string
	Payload int
	ST      int
	DX      int
	IQ      int
	HT      int
	TheGen  int
	Credits int
	Gold    int
	CanSell bool

	// From the client's catalog (registerItem): weapon mount point and stock per world
	Mount       int
	MaxPerWorld int
}

// AttribTotal is the sum of the base attributes (every bot shares the same total).
func (i Item) AttribTotal() int { return i.ST + i.DX + i.IQ + i.HT }

var ErrNotFound = errors.New("item not found")

const itemColumns = "id, class, COALESCE(name, ''), CAST(COALESCE(payload, 0) AS INTEGER), st, dx, iq, ht, the_gen, credits, gold, can_sell, mount, max_per_world"

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(row scanner) (Item, error) {
	var item Item
	var canSell int

	err := row.Scan(&item.Id, &item.Class, &item.Name, &item.Payload, &item.ST, &item.DX, &item.IQ, &item.HT, &item.TheGen, &item.Credits, &item.Gold, &canSell, &item.Mount, &item.MaxPerWorld)
	item.CanSell = canSell != 0

	return item, err
}

// Querier is satisfied by both *sql.DB and *sql.Tx.
type Querier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

func Get(id int) (Item, error) {
	return GetWith(database.DB, id)
}

func GetWith(q Querier, id int) (Item, error) {
	item, err := scanItem(q.QueryRow("SELECT "+itemColumns+" FROM items WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return item, err
}

func GetAll() ([]Item, error) {
	return query(database.DB, "SELECT "+itemColumns+" FROM items ORDER BY id")
}

// Starters are the free first generation items handed out on registration.
func Starters(q Querier) ([]Item, error) {
	return query(q, "SELECT "+itemColumns+" FROM items WHERE credits = 0 AND gold = 0 AND the_gen = 1 ORDER BY id")
}

// Owned returns the user's items keyed by item id, along with whether they are equipped.
func Owned(q Querier, userId int) (map[int]Item, map[int]bool, error) {
	rows, err := q.Query("SELECT "+prefixed("i")+", ui.enabled FROM user_items ui JOIN items i ON i.id = ui.item_id WHERE ui.user_id = ?", userId)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	owned := make(map[int]Item)
	equipped := make(map[int]bool)

	for rows.Next() {
		var item Item
		var canSell, enabled int

		if err := rows.Scan(&item.Id, &item.Class, &item.Name, &item.Payload, &item.ST, &item.DX, &item.IQ, &item.HT, &item.TheGen, &item.Credits, &item.Gold, &canSell, &item.Mount, &item.MaxPerWorld, &enabled); err != nil {
			return nil, nil, err
		}

		item.CanSell = canSell != 0
		owned[item.Id] = item
		equipped[item.Id] = enabled != 0
	}

	return owned, equipped, rows.Err()
}

func prefixed(alias string) string {
	return fmt.Sprintf("%[1]s.id, %[1]s.class, COALESCE(%[1]s.name, ''), CAST(COALESCE(%[1]s.payload, 0) AS INTEGER), %[1]s.st, %[1]s.dx, %[1]s.iq, %[1]s.ht, %[1]s.the_gen, %[1]s.credits, %[1]s.gold, %[1]s.can_sell, %[1]s.mount, %[1]s.max_per_world", alias)
}

func query(q Querier, stmt string, args ...any) ([]Item, error) {
	rows, err := q.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

// Owners returns how many users have the item (for the per world stock).
func Owners(q Querier, itemId int) (int, error) {
	var count int
	err := q.QueryRow("SELECT COUNT(*) FROM user_items WHERE item_id = ?", itemId).Scan(&count)
	return count, err
}

// UnlimitedStock is reported for items without a per world limit.
const UnlimitedStock = 99999

// Available is how many more copies can still be sold.
func (i Item) Available(owners int) int {
	if i.MaxPerWorld <= 0 {
		return UnlimitedStock
	}
	if owners >= i.MaxPerWorld {
		return 0
	}
	return i.MaxPerWorld - owners
}

// Grant gives the item to the user with its base attributes (no ownership checks).
func Grant(q Querier, userId int, item Item, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	_, err := q.Exec("INSERT INTO user_items (item_id, user_id, st, dx, iq, ht, enabled) VALUES (?, ?, ?, ?, ?, ?, ?)",
		item.Id, userId, item.ST, item.DX, item.IQ, item.HT, enabledInt)

	return err
}
