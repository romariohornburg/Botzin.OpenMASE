package database

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
)

var DB *sql.DB

// ItemsSchemaChanged is set when columns were added to the items table, so the catalog must be reloaded.
var ItemsSchemaChanged bool

func contactsTable(db *sql.DB) {
	sqlStmt := `
    CREATE TABLE IF NOT EXISTS contacts (
        id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
        user_id INTEGER NOT NULL,
        contact_id INTEGER NOT NULL,
		status INT DEFAULT 0,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		FOREIGN KEY(contact_id) REFERENCES users(id) ON DELETE CASCADE
    );
    `
	_, err := db.Exec(sqlStmt)
	if err != nil {
		log.Error().Msgf("Error creating contacts table: %s", err)
		return
	}

	log.Info().Msg("Contacts table created successfully")
}

func itemsTable(db *sql.DB) {
	sqlStmt := `
    CREATE TABLE IF NOT EXISTS items (
        id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
        class INT DEFAULT 0,
		name TEXT,
        description TEXT,
		payload TEXT,
		st INT DEFAULT 0,
		dx INT DEFAULT 0,
		iq INT DEFAULT 0,
		ht INT DEFAULT 0,
		the_gen INT DEFAULT 0,
		credits INT DEFAULT 0,
		gold INT DEFAULT 0,
		can_sell INT DEFAULT 0
    );
    `
	_, err := db.Exec(sqlStmt)
	if err != nil {
		log.Error().Msgf("Error creating items table: %s", err)
		return
	}

	// Catalog data taken from the client (registerItem); a reload fills them in
	if ensureColumn(db, "items", "mount", "INT DEFAULT 0") {
		ItemsSchemaChanged = true
	}
	if ensureColumn(db, "items", "max_per_world", "INT DEFAULT 0") {
		ItemsSchemaChanged = true
	}

	log.Info().Msg("Items table created successfully")
}
func usersTable(db *sql.DB) {
	sqlStmt := `
    CREATE TABLE IF NOT EXISTS users (
        id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
        username TEXT,
		username_md5 TEXT,
		password_md5 TEXT,
		xp INT DEFAULT 0,
		st INT DEFAULT 0,
		dx INT DEFAULT 0,
		iq INT DEFAULT 0,
		ht INT DEFAULT 0,
		points INT DEFAULT 0,
		credits INT DEFAULT 0,
		gold INT DEFAULT 0,
		ranking INT DEFAULT 0,
		totalRK INT DEFAULT 0,
		level INT DEFAULT 0,
		pmx INT DEFAULT 0,
		clanTag TEXT
    );
    `
	_, err := db.Exec(sqlStmt)
	if err != nil {
		log.Error().Msgf("Error creating users table: %s", err)
		return
	}

	// Columns added after the first release
	ensureColumn(db, "users", "banned", "INT DEFAULT 0")
	ensureColumn(db, "users", "patent", "INT DEFAULT 0")
	ensureColumn(db, "users", "matches", "INT DEFAULT 0")
	ensureColumn(db, "users", "wins", "INT DEFAULT 0")
	ensureColumn(db, "users", "kills", "INT DEFAULT 0")
	ensureColumn(db, "users", "deaths", "INT DEFAULT 0")
	ensureColumn(db, "users", "created_at", "INT DEFAULT 0")

	if _, err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS users_username_md5 ON users(username_md5)"); err != nil {
		log.Warn().Msgf("Could not create unique index on users.username_md5 (duplicate accounts?): %s", err)
	}

	log.Info().Msg("Users table created successfully")
}

func userItemsTable(db *sql.DB) {
	sqlStmt := `
    CREATE TABLE IF NOT EXISTS user_items (
        id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
        item_id INTEGER NOT NULL,
        user_id INTEGER NOT NULL,
		xp INT DEFAULT 0,
		st INT DEFAULT 0,
		dx INT DEFAULT 0,
		iq INT DEFAULT 0,
		ht INT DEFAULT 0,
		enabled INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE
    );
    `
	_, err := db.Exec(sqlStmt)
	if err != nil {
		log.Error().Msgf("Error creating user_items table: %s", err)
		return
	}

	log.Info().Msg("UserItems table created successfully")
}

// ensureColumn adds a column to an existing table if it's missing (SQLite has no ADD COLUMN IF NOT EXISTS).
// Returns whether the column was added.
func ensureColumn(db *sql.DB, table string, column string, definition string) bool {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		log.Error().Msgf("Error reading %s schema: %s", table, err)
		return false
	}

	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var defaultValue sql.NullString

		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err == nil && strings.EqualFold(name, column) {
			found = true
		}
	}
	rows.Close()

	if found {
		return false
	}

	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)); err != nil {
		log.Error().Msgf("Error adding column %s.%s: %s", table, column, err)
		return false
	}

	log.Info().Msgf("Added column %s.%s", table, column)
	return true
}

func Initialize(path string) error {
	var err error

	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=WAL", path)
	DB, err = sql.Open("sqlite3", dsn)

	if err != nil {
		log.Error().Msgf("Error initializing SQLite database: %s", err)
		return err
	}

	if err = DB.Ping(); err != nil {
		log.Error().Msgf("Error opening SQLite database: %s", err)
		return err
	}

	usersTable(DB)
	contactsTable(DB)
	itemsTable(DB)
	userItemsTable(DB)

	return nil
}

// findItemsSQL looks for the items seed in the usual places when no path is configured.
func findItemsSQL(path string) string {
	if path != "" {
		return path
	}

	for _, candidate := range []string{"sql/items.sql", "../sql/items.sql", "items.sql"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// LoadItems seeds the items table from the SQL file. Unless force is set, it only runs on an empty table.
// When forced, existing items are replaced (user inventories are kept).
func LoadItems(path string, force bool) (int, error) {
	if !force {
		var count int
		if err := DB.QueryRow("SELECT COUNT(*) FROM items").Scan(&count); err != nil {
			return 0, err
		}
		if count > 0 {
			return 0, nil
		}
	}

	path = findItemsSQL(path)
	if path == "" {
		return 0, fmt.Errorf("items.sql not found, set ITEMS_SQL_PATH")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	loaded := 0
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		// Allow reloading over existing rows (foreign keys are off, so REPLACE does not cascade into user_items)
		stmt = strings.Replace(stmt, "INSERT INTO", "INSERT OR REPLACE INTO", 1)

		if _, err := tx.Exec(stmt); err != nil {
			return 0, fmt.Errorf("%s: %w", stmt, err)
		}
		loaded++
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	log.Info().Msgf("Loaded %d statements from %s", loaded, path)

	return loaded, nil
}
