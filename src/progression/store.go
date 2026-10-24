package progression

import (
	"database/sql"
	"errors"

	"github.com/iagoMAO/Botzin.OpenMASE/database"
	"github.com/rs/zerolog/log"
)

// Result is what a single player got out of a match.
type Result struct {
	Entry   ScoreEntry
	Rewards Rewards
	Known   bool // false when the GUID isn't a registered user (nothing was applied)

	XP            int
	Level         int
	PreviousLevel int
	Patent        int
	Credits       int
	Ranking       int
	TotalRK       int
}

func (r Result) LevelUp() bool { return r.Level > r.PreviousLevel }

// ApplyReport persists the rewards of every player of a match in a single transaction
// and refreshes the global ranking.
func ApplyReport(report ScoreReport, rules Rules) ([]Result, error) {
	tx, err := database.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	results := make([]Result, 0, len(report.Entries))

	for _, entry := range report.Entries {
		result := Result{Entry: entry, Rewards: Compute(entry, rules)}

		err := tx.QueryRow("SELECT xp, level, credits FROM users WHERE id = ?", entry.GUID).
			Scan(&result.XP, &result.PreviousLevel, &result.Credits)

		if errors.Is(err, sql.ErrNoRows) {
			results = append(results, result)
			continue
		}
		if err != nil {
			return nil, err
		}

		result.Known = true
		result.XP += result.Rewards.XP
		result.Credits += result.Rewards.Credits
		result.Level = LevelForXP(result.XP)
		result.Patent = PatentForLevel(result.Level)

		won := 0
		if entry.Winner && !entry.Dropped {
			won = 1
		}

		_, err = tx.Exec(`
			UPDATE users SET
				xp = ?, level = ?, patent = ?, credits = ?,
				points = MAX(0, points + ?), matches = matches + 1, wins = wins + ?,
				kills = kills + ?, deaths = deaths + ?
			WHERE id = ?`,
			result.XP, result.Level, result.Patent, result.Credits,
			result.Rewards.Score, won, entry.Kills, report.PlayersPerClient-entry.Botz, entry.GUID)
		if err != nil {
			return nil, err
		}

		results = append(results, result)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if err := RecalculateRanking(); err != nil {
		log.Error().Msgf("Failed to recalculate ranking: %s", err)
	}

	for i := range results {
		if results[i].Known {
			database.DB.QueryRow("SELECT ranking, totalRK FROM users WHERE id = ?", results[i].Entry.GUID).
				Scan(&results[i].Ranking, &results[i].TotalRK)
		}
	}

	return results, nil
}

// AddXP grants (or removes) XP outside of a match, keeping level & patent consistent.
func AddXP(userId int, amount int) (int, int, error) {
	var xp int
	if err := database.DB.QueryRow("SELECT xp FROM users WHERE id = ?", userId).Scan(&xp); err != nil {
		return 0, 0, err
	}

	xp += amount
	if xp < 0 {
		xp = 0
	}

	level := LevelForXP(xp)

	_, err := database.DB.Exec("UPDATE users SET xp = ?, level = ?, patent = ? WHERE id = ?", xp, level, PatentForLevel(level), userId)
	if err != nil {
		return 0, 0, err
	}

	return xp, level, RecalculateRanking()
}

// RecalculateRanking orders every non banned player by points (then XP) and
// stores the position plus the total number of ranked players.
func RecalculateRanking() error {
	_, err := database.DB.Exec(`
		UPDATE users SET
			ranking = COALESCE((
				SELECT position FROM (
					SELECT id, ROW_NUMBER() OVER (ORDER BY points DESC, xp DESC, id ASC) AS position
					FROM users WHERE banned = 0
				) ranked WHERE ranked.id = users.id
			), 0),
			totalRK = (SELECT COUNT(*) FROM users WHERE banned = 0)`)

	return err
}

type RankEntry struct {
	Position int
	Nick     string
	Points   int
	Level    int
}

func TopRanking(limit int) ([]RankEntry, error) {
	rows, err := database.DB.Query("SELECT ranking, username, points, level FROM users WHERE banned = 0 AND ranking > 0 ORDER BY ranking LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RankEntry
	for rows.Next() {
		var entry RankEntry
		if err := rows.Scan(&entry.Position, &entry.Nick, &entry.Points, &entry.Level); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}

	return out, rows.Err()
}
