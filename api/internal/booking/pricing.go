package booking

import (
	"errors"
	"fmt"
	"time"
)

// ErrOutsideOpeningHours means part of the requested time is before opening or after closing.
var ErrOutsideOpeningHours = errors.New("the booking falls outside opening hours")

// slotStep is the grid every booking is aligned to; prices are charged per step.
const slotStep = 30 * time.Minute

// PriceRule is one band of the venue's price list, in minutes after local midnight.
type PriceRule struct {
	Sport          string
	StartsMinute   int
	EndsMinute     int
	HourlyPriceIDR int32
}

// Price charges each half hour of [localStart, localEnd) at the rule that covers it, so a
// booking from 16.00 to 18.00 pays one hour at the day rate and one at the evening rate.
func Price(rules []PriceRule, sport string, localStart, localEnd time.Time) (int32, error) {
	if !localEnd.After(localStart) {
		return 0, ErrInvalidPeriod
	}
	var total int32
	for step := localStart; step.Before(localEnd); step = step.Add(slotStep) {
		minute := step.Hour()*60 + step.Minute()
		rule, ok := ruleAt(rules, sport, minute)
		if !ok {
			return 0, fmt.Errorf("%w: no %s price at %s", ErrOutsideOpeningHours, sport,
				step.Format("15.04"))
		}
		total += rule.HourlyPriceIDR / 2
	}
	return total, nil
}

func ruleAt(rules []PriceRule, sport string, minute int) (PriceRule, bool) {
	for _, rule := range rules {
		if rule.Sport == sport && rule.StartsMinute <= minute && minute < rule.EndsMinute {
			return rule, true
		}
	}
	return PriceRule{}, false
}

// Deposit is half the price, rounded up to the next thousand Rupiah so the amount is easy to
// read and pay.
func Deposit(price int32) int32 {
	half := (price + 1) / 2
	return (half + 999) / 1000 * 1000
}
