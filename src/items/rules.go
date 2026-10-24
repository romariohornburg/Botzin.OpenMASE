package items

import (
	"errors"
	"fmt"
)

var (
	ErrNotOwned        = errors.New("item not owned")
	ErrDuplicateItem   = errors.New("item equipped twice")
	ErrTooManyBots     = errors.New("more than one bot equipped")
	ErrPayloadExceeded = errors.New("bot payload exceeded")
	ErrNotABot         = errors.New("item is not a bot")
	ErrAttribRange     = errors.New("attribute out of range")
	ErrAttribBudget    = errors.New("attribute points exceed the budget")
)

// PayloadUsage returns the capacity of the equipped bot and how much the other items use.
// Bots carry their capacity in the payload column, every other item consumes payload.
func PayloadUsage(setup []Item) (capacity int, used int) {
	for _, item := range setup {
		if item.Class == ClassBot {
			capacity = item.Payload
		} else {
			used += item.Payload
		}
	}
	return capacity, used
}

// ValidateSetup checks an equipment list against the user's inventory.
func ValidateSetup(owned map[int]Item, itemIds []int) ([]Item, error) {
	seen := make(map[int]bool)
	bots := 0

	var setup []Item

	for _, id := range itemIds {
		item, ok := owned[id]
		if !ok {
			return nil, fmt.Errorf("%w: %d", ErrNotOwned, id)
		}

		if seen[id] {
			return nil, fmt.Errorf("%w: %d", ErrDuplicateItem, id)
		}
		seen[id] = true

		if item.Class == ClassBot {
			bots++
		}

		setup = append(setup, item)
	}

	if bots > 1 {
		return nil, ErrTooManyBots
	}

	// Without a bot nothing is playable, so there's no capacity to abuse
	if bots == 1 {
		capacity, used := PayloadUsage(setup)
		if used > capacity {
			return nil, fmt.Errorf("%w: %d/%d", ErrPayloadExceeded, used, capacity)
		}
	}

	return setup, nil
}

type AttribLimits struct {
	Min            int
	Max            int
	PointsPerLevel int
}

// ValidateAttribs checks a bot's attribute distribution. The sum may not exceed the
// bot's base total plus the points earned by leveling up.
func ValidateAttribs(bot Item, level int, st, dx, iq, ht int, limits AttribLimits) error {
	if bot.Class != ClassBot {
		return ErrNotABot
	}

	for _, value := range []int{st, dx, iq, ht} {
		if value < limits.Min || value > limits.Max {
			return fmt.Errorf("%w: %d not in [%d, %d]", ErrAttribRange, value, limits.Min, limits.Max)
		}
	}

	if level < 0 {
		level = 0
	}

	budget := bot.AttribTotal() + level*limits.PointsPerLevel
	if total := st + dx + iq + ht; total > budget {
		return fmt.Errorf("%w: %d > %d", ErrAttribBudget, total, budget)
	}

	return nil
}
