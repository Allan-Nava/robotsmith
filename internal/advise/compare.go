package advise

import (
	"fmt"
	"sort"
)

// Snapshot is a previous run, reduced to what a comparison needs. The caller builds it from a
// stored `--json` document, so this package keeps knowing nothing about JSON.
type Snapshot struct {
	Total  int64
	Shares map[string]float64 // crawler name → share of the observed total, in percent
}

// Direction is how a crawler moved between two observations.
type Direction int

const (
	Steady Direction = iota
	Grew
	Shrank
	Appeared
	Vanished
)

func (d Direction) String() string {
	return [...]string{"steady", "grew", "shrank", "appeared", "vanished"}[d]
}

// Movement is one crawler's change between the two runs.
type Movement struct {
	Name      string
	Direction Direction
	Was, Now  float64 // shares, in percent
	Factor    float64 // Now / Was; 0 when it appeared, +Inf is avoided by construction
}

// Comparison is the whole delta, heaviest movement first.
type Comparison struct {
	PreviousTotal int64
	Movements     []Movement
}

// MovedThreshold is how much a share has to change before it counts as movement rather than noise.
// Traffic wobbles: calling every 1% wobble a trend would make the report unreadable and, worse,
// train the reader to skim past it.
const MovedThreshold = 1.25

// GrowthPromotion is the factor at which a crawler too small to be worth a line becomes worth a
// look anyway. ⚠️ This is the whole point of comparing: 0.4% flat for a year and 0.4% quadrupling
// this month get the same answer from a share alone, and only the second one is a decision.
const GrowthPromotion = 2.0

// compare builds the delta between a previous snapshot and the decisions just made. It also reports
// crawlers that vanished: an advice file still blocking something that stopped coming is stale, and
// nobody notices a rule that no longer does anything.
func compare(prev *Snapshot, decisions []Decision, seen map[string]float64) *Comparison {
	c := &Comparison{PreviousTotal: prev.Total}
	now := map[string]float64{}
	for name, share := range seen {
		now[name] = share
	}
	for _, d := range decisions {
		now[d.Name] = d.Share
	}

	names := make([]string, 0, len(now)+len(prev.Shares))
	for n := range now {
		names = append(names, n)
	}
	for n := range prev.Shares {
		if _, ok := now[n]; !ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	for _, n := range names {
		was, wasThere := prev.Shares[n]
		is, isThere := now[n]
		m := Movement{Name: n, Was: was, Now: is}
		switch {
		case !wasThere:
			m.Direction = Appeared
		case !isThere || is == 0:
			m.Direction, m.Now = Vanished, 0
		default:
			m.Factor = is / was
			switch {
			case m.Factor >= MovedThreshold:
				m.Direction = Grew
			case m.Factor <= 1/MovedThreshold:
				m.Direction = Shrank
			default:
				m.Direction = Steady
			}
		}
		c.Movements = append(c.Movements, m)
	}
	// Heaviest first: the reader's attention is the scarce resource, and a crawler at 9% moving
	// matters more than one at 0.05% doing the same.
	sort.SliceStable(c.Movements, func(i, j int) bool {
		return maxf(c.Movements[i].Was, c.Movements[i].Now) > maxf(c.Movements[j].Was, c.Movements[j].Now)
	})
	return c
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// growthReason is what a promoted crawler gets told about itself: the numbers, not an adjective.
func growthReason(was, now, factor float64) string {
	return fmt.Sprintf("still small (%.1f%%) but growing fast: was %.1f%%, now %.1f%% (×%.3g) — "+
		"a share alone cannot tell this apart from one that has been flat for a year", now, was, now, factor)
}
