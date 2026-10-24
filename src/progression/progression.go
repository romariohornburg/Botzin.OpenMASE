package progression

import (
	"errors"
	"fmt"

	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"
)

const (
	MaxLevel  = 99
	MaxPatent = 10

	// Levels needed per patent tier
	LevelsPerPatent = 5

	// Sanity limits for what a match host may report
	maxClientsPerTeam   = 4
	maxPlayersPerClient = 10

	headerFields = 6
	entryFields  = 4
)

var ErrMalformedReport = errors.New("malformed score report")

// ScoreEntry is one client of the match as reported by the host.
type ScoreEntry struct {
	GUID    int
	Kills   int
	Botz    int // bots still alive at the end of the match
	Dropped bool
	Winner  bool
}

// ScoreReport mirrors MASERequestScore() in the 1.17 client. The match host sends:
//
//	\t LocalClientGUID \t Mission.dbIndex \t TurnTimePos \t Wind \t InitialNumClients/2 \t PlayersPerClient
//
// followed by one "\t guid \t numKills \t numPlayers \t dropped" group per client (all SCR packed),
// listing the winning team first. The winner team is always set (ties are broken at random),
// so the first ClientsPerTeam entries are the winners.
type ScoreReport struct {
	HostGUID         int
	MissionIndex     int
	TurnTimePos      int
	Wind             int
	ClientsPerTeam   int
	PlayersPerClient int
	Entries          []ScoreEntry
}

// ParseScoreReport reads the tab separated fields of a ScoreRequest (protocol.NetworkMessage.Fields()).
func ParseScoreReport(fields [][]byte) (ScoreReport, error) {
	// The request starts with a tab right after the status byte
	if len(fields) > 0 && len(fields[0]) == 0 {
		fields = fields[1:]
	}

	if len(fields) < headerFields || (len(fields)-headerFields)%entryFields != 0 {
		return ScoreReport{}, fmt.Errorf("%w: %d fields", ErrMalformedReport, len(fields))
	}

	value := func(i int) int { return data.SCR_StrToInt(fields[i]) }

	report := ScoreReport{
		HostGUID:         value(0),
		MissionIndex:     value(1),
		TurnTimePos:      value(2),
		Wind:             value(3),
		ClientsPerTeam:   value(4),
		PlayersPerClient: value(5),
	}

	if report.ClientsPerTeam < 1 || report.ClientsPerTeam > maxClientsPerTeam ||
		report.PlayersPerClient < 1 || report.PlayersPerClient > maxPlayersPerClient {
		return ScoreReport{}, fmt.Errorf("%w: %d clients per team, %d players per client", ErrMalformedReport, report.ClientsPerTeam, report.PlayersPerClient)
	}

	count := (len(fields) - headerFields) / entryFields
	if count > 2*report.ClientsPerTeam {
		return ScoreReport{}, fmt.Errorf("%w: %d clients for %d per team", ErrMalformedReport, count, report.ClientsPerTeam)
	}

	// Nobody can kill more bots than the other team has
	maxKills := report.ClientsPerTeam * report.PlayersPerClient
	seen := make(map[int]bool)

	for i := 0; i < count; i++ {
		base := headerFields + i*entryFields

		entry := ScoreEntry{
			GUID:    value(base),
			Kills:   clamp(value(base+1), 0, maxKills),
			Botz:    clamp(value(base+2), 0, report.PlayersPerClient),
			Dropped: value(base+3) != 0,
			Winner:  i < report.ClientsPerTeam,
		}

		if entry.GUID <= 0 || seen[entry.GUID] {
			return ScoreReport{}, fmt.Errorf("%w: invalid or duplicated guid %d", ErrMalformedReport, entry.GUID)
		}
		seen[entry.GUID] = true

		report.Entries = append(report.Entries, entry)
	}

	return report, nil
}

// Rules holds the tunables that come from the config.
type Rules struct {
	KillPoints      int
	AlivePoints     int
	WinPoints       int
	DropPenalty     int
	CreditsPerPoint int
}

type Rewards struct {
	Score   int // may be negative (dropping a match is penalized)
	XP      int
	Credits int
}

// Compute returns what a match entry is worth. The original formula is unknown
// (the client only displays what the server answers), so this one is ours.
func Compute(entry ScoreEntry, rules Rules) Rewards {
	if entry.Dropped {
		return Rewards{Score: -rules.DropPenalty}
	}

	score := entry.Kills*rules.KillPoints + entry.Botz*rules.AlivePoints
	if entry.Winner {
		score += rules.WinPoints
	}

	return Rewards{Score: score, XP: score, Credits: score * rules.CreditsPerPoint}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// XPForLevel is the total XP needed to reach a level: 50 * n * (n + 1).
// Level 1 = 100 XP, level 10 = 5.500 XP, level 50 = 127.500 XP.
func XPForLevel(level int) int {
	return 50 * level * (level + 1)
}

func LevelForXP(xp int) int {
	level := 0
	for level < MaxLevel && xp >= XPForLevel(level+1) {
		level++
	}
	return level
}

func PatentForLevel(level int) int {
	return clamp(level/LevelsPerPatent, 0, MaxPatent)
}
