package progression

import (
	"errors"
	"testing"

	"github.com/iagoMAO/Botzin.OpenMASE/utils/data"
)

var rules = Rules{KillPoints: 10, AlivePoints: 5, WinPoints: 50, DropPenalty: 20, CreditsPerPoint: 10}

// report builds the fields exactly like MASERequestScore() in the client:
// a leading empty field (the request starts with a tab), the header, then 4 fields per client.
func report(header []int, clients ...[]int) [][]byte {
	fields := [][]byte{{}}
	for _, v := range header {
		fields = append(fields, data.SCR_PackInt(v))
	}
	for _, client := range clients {
		for _, v := range client {
			fields = append(fields, data.SCR_PackInt(v))
		}
	}
	return fields
}

func TestParseScoreReport(t *testing.T) {
	// host 7, mission 3, turn time 120, wind 2, 1 client per team, 3 bots per client
	got, err := ParseScoreReport(report([]int{7, 3, 120, 2, 1, 3}, []int{7, 2, 1, 0}, []int{9, 0, 0, 1}))
	if err != nil {
		t.Fatal(err)
	}

	if got.HostGUID != 7 || got.MissionIndex != 3 || got.ClientsPerTeam != 1 || got.PlayersPerClient != 3 || len(got.Entries) != 2 {
		t.Fatalf("unexpected header %+v", got)
	}

	want := []ScoreEntry{
		{GUID: 7, Kills: 2, Botz: 1, Winner: true},
		{GUID: 9, Kills: 0, Botz: 0, Dropped: true},
	}
	for i := range want {
		if got.Entries[i] != want[i] {
			t.Errorf("entry %d: got %+v, want %+v", i, got.Entries[i], want[i])
		}
	}
}

func TestParseScoreReportClampsAndRejects(t *testing.T) {
	// Kills above what the other team has and more bots than allowed are clamped
	got, err := ParseScoreReport(report([]int{7, 1, 0, 0, 2, 3}, []int{7, 99, 99, 0}))
	if err != nil {
		t.Fatal(err)
	}
	if got.Entries[0].Kills != 6 || got.Entries[0].Botz != 3 {
		t.Fatalf("not clamped: %+v", got.Entries[0])
	}

	bad := map[string][][]byte{
		"short header":        report([]int{7, 1, 0}),
		"partial client":      report([]int{7, 1, 0, 0, 1, 3}, []int{7, 1}),
		"too many clients":    report([]int{7, 1, 0, 0, 1, 3}, []int{7, 0, 0, 0}, []int{8, 0, 0, 0}, []int{9, 0, 0, 0}),
		"duplicated client":   report([]int{7, 1, 0, 0, 1, 3}, []int{7, 0, 0, 0}, []int{7, 0, 0, 0}),
		"invalid team size":   report([]int{7, 1, 0, 0, 0, 3}),
		"invalid player size": report([]int{7, 1, 0, 0, 1, 50}),
		"zero guid":           report([]int{7, 1, 0, 0, 1, 3}, []int{0, 0, 0, 0}),
	}

	for name, fields := range bad {
		if _, err := ParseScoreReport(fields); !errors.Is(err, ErrMalformedReport) {
			t.Errorf("%s: expected ErrMalformedReport, got %v", name, err)
		}
	}
}

func TestCompute(t *testing.T) {
	cases := []struct {
		entry ScoreEntry
		want  Rewards
	}{
		{ScoreEntry{Kills: 2, Botz: 1, Winner: true}, Rewards{Score: 75, XP: 75, Credits: 750}},
		{ScoreEntry{Kills: 1}, Rewards{Score: 10, XP: 10, Credits: 100}},
		{ScoreEntry{Kills: 3, Botz: 2, Winner: true, Dropped: true}, Rewards{Score: -20}},
	}

	for _, c := range cases {
		if got := Compute(c.entry, rules); got != c.want {
			t.Errorf("%+v: got %+v, want %+v", c.entry, got, c.want)
		}
	}
}

func TestLevels(t *testing.T) {
	cases := map[int]int{0: 0, 99: 0, 100: 1, 299: 1, 300: 2, 5500: 10, 1 << 30: MaxLevel}

	for xp, level := range cases {
		if got := LevelForXP(xp); got != level {
			t.Errorf("xp %d: got level %d, want %d", xp, got, level)
		}
	}

	if PatentForLevel(0) != 0 || PatentForLevel(12) != 2 || PatentForLevel(MaxLevel) != MaxPatent {
		t.Error("unexpected patent")
	}
}
