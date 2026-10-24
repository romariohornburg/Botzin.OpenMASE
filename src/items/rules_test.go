package items

import (
	"errors"
	"testing"
)

var (
	babyBoy  = Item{Id: 17, Class: ClassBot, Payload: 10, ST: 100, DX: 100, IQ: 100, HT: 100}
	rifle    = Item{Id: 1, Class: ClassWeapon, Payload: 1}
	missile  = Item{Id: 3, Class: ClassWeapon, Payload: 3}
	cannon   = Item{Id: 6, Class: ClassWeapon, Payload: 3}
	rocket   = Item{Id: 25, Class: ClassWeapon, Payload: 3}
	nova     = Item{Id: 26, Class: ClassWeapon, Payload: 3}
	babyFat  = Item{Id: 19, Class: ClassBot, Payload: 14}
	inventry = map[int]Item{17: babyBoy, 1: rifle, 3: missile, 6: cannon, 25: rocket, 26: nova, 19: babyFat}
)

func TestValidateSetup(t *testing.T) {
	cases := []struct {
		name string
		ids  []int
		want error
	}{
		{"ok", []int{17, 1, 3, 6}, nil},
		{"empty", nil, nil},
		{"not owned", []int{17, 99}, ErrNotOwned},
		{"duplicate", []int{17, 1, 1}, ErrDuplicateItem},
		{"two bots", []int{17, 19}, ErrTooManyBots},
		{"payload exceeded", []int{17, 1, 3, 6, 25, 26}, ErrPayloadExceeded},
		{"bigger bot fits", []int{19, 1, 3, 6, 25}, nil},
	}

	for _, c := range cases {
		_, err := ValidateSetup(inventry, c.ids)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestValidateAttribs(t *testing.T) {
	limits := AttribLimits{Min: 0, Max: 250, PointsPerLevel: 2}

	cases := []struct {
		name           string
		bot            Item
		level          int
		st, dx, iq, ht int
		want           error
	}{
		{"base", babyBoy, 0, 100, 100, 100, 100, nil},
		{"redistributed", babyBoy, 0, 160, 40, 100, 100, nil},
		{"over budget", babyBoy, 0, 101, 100, 100, 100, ErrAttribBudget},
		{"level bonus", babyBoy, 5, 110, 100, 100, 100, nil},
		{"negative", babyBoy, 0, -1, 100, 100, 100, ErrAttribRange},
		{"over max", babyBoy, 99, 251, 100, 100, 100, ErrAttribRange},
		{"not a bot", rifle, 0, 0, 0, 0, 0, ErrNotABot},
	}

	for _, c := range cases {
		err := ValidateAttribs(c.bot, c.level, c.st, c.dx, c.iq, c.ht, limits)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}
