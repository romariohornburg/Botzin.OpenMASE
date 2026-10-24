package utils

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

type Config struct {
	MASE_PORT                 string
	HB_PORT                   string
	BUDDY_PORT                string
	SERVERLIST_PORT           string
	REGISTER_PORT             string
	XTEA_KEY                  string
	WELCOME_BROADCAST_MESSAGE string

	DB_PATH        string
	ITEMS_SQL_PATH string

	// Validate the Buddy login against the random token handed out on MASE login.
	// When disabled, the Buddy login is only accepted from the same IP as the MASE session.
	SESSION_TOKEN_AUTH bool

	STARTING_CREDITS int
	STARTING_GOLD    int

	// End of match rewards (see progression.Compute)
	SCORE_KILL_POINTS          int
	SCORE_ALIVE_POINTS         int
	SCORE_WIN_POINTS           int
	SCORE_DROP_PENALTY         int
	CREDITS_PER_POINT          int
	MIN_SCORE_INTERVAL_SECONDS int
	SCORE_NOTIFY               bool

	SELL_RATIO_PERCENT      int
	ATTRIB_MIN              int
	ATTRIB_MAX              int
	ATTRIB_POINTS_PER_LEVEL int
}

var (
	cfg     Config
	cfgOnce sync.Once
)

// GetConfig loads the .env file (once) and returns the cached configuration.
func GetConfig() Config {
	cfgOnce.Do(func() {
		if err := godotenv.Load(); err != nil {
			log.Warn().Msg("No .env file found, using environment variables only.")
		}

		cfg = Config{
			MASE_PORT:                 getString("MASE_PORT", "33600"),
			HB_PORT:                   getString("HB_PORT", "27999"),
			BUDDY_PORT:                getString("BUDDY_PORT", "33650"),
			SERVERLIST_PORT:           getString("SERVERLIST_PORT", "28002"),
			REGISTER_PORT:             getString("REGISTER_PORT", ""),
			XTEA_KEY:                  getString("XTEA_KEY", ""),
			WELCOME_BROADCAST_MESSAGE: getString("WELCOME_BROADCAST_MESSAGE", ""),

			DB_PATH:        getString("DB_PATH", "./botzin.db"),
			ITEMS_SQL_PATH: getString("ITEMS_SQL_PATH", ""),

			SESSION_TOKEN_AUTH: getBool("SESSION_TOKEN_AUTH", true),

			STARTING_CREDITS: getInt("STARTING_CREDITS", 50000),
			STARTING_GOLD:    getInt("STARTING_GOLD", 0),

			SCORE_KILL_POINTS:          getInt("SCORE_KILL_POINTS", 10),
			SCORE_ALIVE_POINTS:         getInt("SCORE_ALIVE_POINTS", 5),
			SCORE_WIN_POINTS:           getInt("SCORE_WIN_POINTS", 50),
			SCORE_DROP_PENALTY:         getInt("SCORE_DROP_PENALTY", 20),
			CREDITS_PER_POINT:          getInt("CREDITS_PER_POINT", 10),
			MIN_SCORE_INTERVAL_SECONDS: getInt("MIN_SCORE_INTERVAL_SECONDS", 60),
			SCORE_NOTIFY:               getBool("SCORE_NOTIFY", false),

			SELL_RATIO_PERCENT:      getInt("SELL_RATIO_PERCENT", 50),
			ATTRIB_MIN:              getInt("ATTRIB_MIN", 0),
			ATTRIB_MAX:              getInt("ATTRIB_MAX", 250),
			ATTRIB_POINTS_PER_LEVEL: getInt("ATTRIB_POINTS_PER_LEVEL", 2),
		}
	})

	return cfg
}

// SetConfig overrides the cached configuration (used by tests).
func SetConfig(c Config) {
	cfgOnce.Do(func() {})
	cfg = c
}

func getString(key string, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}

	i, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		log.Warn().Msgf("Invalid integer for %s: %q, using %d", key, v, def)
		return def
	}
	return i
}

func getBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}

	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		log.Warn().Msgf("Invalid boolean for %s: %q, using %t", key, v, def)
		return def
	}
	return b
}
