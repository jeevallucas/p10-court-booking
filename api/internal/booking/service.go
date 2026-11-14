package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

var (
	// ErrInvalidRequest wraps input that is well-formed but not acceptable, with the reason.
	ErrInvalidRequest = errors.New("invalid request")
	// ErrInThePast means the requested time has already started.
	ErrInThePast = errors.New("the requested time has already started")
	// ErrNotFound means no booking or court has that identifier.
	ErrNotFound = errors.New("not found")
	// ErrInvalidTransition means the booking's current status does not allow the change.
	ErrInvalidTransition = errors.New("the booking's status does not allow this change")
)

// Service applies the booking rules for one venue. Now is the clock for "has it started yet"
// checks; hold expiry uses the database clock.
type Service struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

// View is a booking with its court, as the API shows it.
type View struct {
	store.Booking
	CourtName string
	Sport     string
}

// Court is one court of the venue.
type Court struct {
	ID    pgtype.UUID
	Name  string
	Sport string
}

// Slot is one bookable hour on one court.
type Slot struct {
	CourtID    pgtype.UUID
	Start, End time.Time
	PriceIDR   int32
	Available  bool
}

type venue struct {
	id            pgtype.UUID
	location      *time.Location
	opens, closes int // minutes after local midnight
	courts        []Court
	rules         []PriceRule
}

func (s *Service) loadVenue(ctx context.Context, q *store.Queries) (venue, error) {
	row, err := q.GetVenue(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return venue{}, fmt.Errorf("%w: no venue is set up; run the seed program", ErrNotFound)
	}
	if err != nil {
		return venue{}, fmt.Errorf("load venue: %w", err)
	}
	location, err := time.LoadLocation(row.Timezone)
	if err != nil {
		return venue{}, fmt.Errorf("venue time zone %q: %w", row.Timezone, err)
	}
	courtRows, err := q.ListCourts(ctx, row.ID)
	if err != nil {
		return venue{}, fmt.Errorf("load courts: %w", err)
	}
	ruleRows, err := q.ListPriceRules(ctx, row.ID)
	if err != nil {
		return venue{}, fmt.Errorf("load price rules: %w", err)
	}
	v := venue{
		id:       row.ID,
		location: location,
		opens:    minuteOfDay(row.OpensAt),
		closes:   minuteOfDay(row.ClosesAt),
	}
	for _, c := range courtRows {
		v.courts = append(v.courts, Court{ID: c.ID, Name: c.Name, Sport: c.Sport})
	}
	for _, r := range ruleRows {
		v.rules = append(v.rules, PriceRule{
			Sport:          r.Sport,
			StartsMinute:   minuteOfDay(r.StartsAt),
			EndsMinute:     minuteOfDay(r.EndsAt),
			HourlyPriceIDR: r.HourlyPriceIdr,
		})
	}
	return v, nil
}

func minuteOfDay(t pgtype.Time) int {
	return int(t.Microseconds / int64(time.Minute/time.Microsecond))
}

// dayBounds returns local midnight to the next midnight for a calendar date.
func (v venue) dayBounds(date time.Time) (time.Time, time.Time) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, v.location)
	return start, start.AddDate(0, 0, 1)
}

func (v venue) court(id pgtype.UUID) (Court, bool) {
	for _, c := range v.courts {
		if c.ID == id {
			return c, true
		}
	}
	return Court{}, false
}

// checkPeriod enforces the half-hour grid and opening hours, in venue time.
func (v venue) checkPeriod(start, end time.Time) error {
	if !end.After(start) {
		return ErrInvalidPeriod
	}
	localStart, localEnd := start.In(v.location), end.In(v.location)
	if localStart.Second() != 0 || localStart.Nanosecond() != 0 || localStart.Minute()%30 != 0 ||
		end.Sub(start)%slotStep != 0 {
		return fmt.Errorf("%w: bookings start and end on the hour or half hour", ErrInvalidRequest)
	}
	startMinute := localStart.Hour()*60 + localStart.Minute()
	endDay := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0,
		v.location)
	endMinute := int(localEnd.Sub(endDay) / time.Minute)
	if startMinute < v.opens || endMinute > v.closes {
		return ErrOutsideOpeningHours
	}
	return nil
}

// Availability lists every hour from opening to closing on each court of the sport.
func (s *Service) Availability(ctx context.Context, date time.Time, sport string) ([]Court, []Slot, error) {
	q := store.New(s.Pool)
	v, err := s.loadVenue(ctx, q)
	if err != nil {
		return nil, nil, err
	}
	dayStart, dayEnd := v.dayBounds(date)
	occupying, err := q.ListOccupyingBookings(ctx, store.ListOccupyingBookingsParams{
		FromTime: pgtype.Timestamptz{Time: dayStart, Valid: true},
		ToTime:   pgtype.Timestamptz{Time: dayEnd, Valid: true},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("load bookings: %w", err)
	}

	now := s.Now()
	var courts []Court
	var slots []Slot
	for _, court := range v.courts {
		if court.Sport != sport {
			continue
		}
		courts = append(courts, court)
		for minute := v.opens; minute+60 <= v.closes; minute += 60 {
			start := dayStart.Add(time.Duration(minute) * time.Minute)
			end := start.Add(time.Hour)
			price, err := Price(v.rules, sport, start, end)
			if err != nil {
				return nil, nil, fmt.Errorf("price %s at %s: %w", court.Name, start.Format("15.04"), err)
			}
			slots = append(slots, Slot{
				CourtID:   court.ID,
				Start:     start,
				End:       end,
				PriceIDR:  price,
				Available: start.After(now) && !occupied(occupying, court.ID, start, end),
			})
		}
	}
	return courts, slots, nil
}

func occupied(bookings []store.ListOccupyingBookingsRow, court pgtype.UUID, start, end time.Time) bool {
	for _, b := range bookings {
		if b.Booking.CourtID == court && b.Booking.Period.Lower.Time.Before(end) &&
			start.Before(b.Booking.Period.Upper.Time) {
			return true
		}
	}
	return false
}

// PlayerHold is what a player asks for: a court, a start on the hour, and one or two hours.
type PlayerHold struct {
	CourtID       pgtype.UUID
	Start         time.Time
	Hours         int
	CustomerName  string
	CustomerPhone string
}

// HoldForPlayer prices the request from the venue's rules (never from the client) and places a
// ten-minute hold.
func (s *Service) HoldForPlayer(ctx context.Context, req PlayerHold) (View, error) {
	if req.Hours < 1 || req.Hours > 2 {
		return View{}, fmt.Errorf("%w: players book one or two hours", ErrInvalidRequest)
	}
	phone, err := NormalizePhone(req.CustomerPhone)
	if err != nil {
		return View{}, err
	}
	v, err := s.loadVenue(ctx, store.New(s.Pool))
	if err != nil {
		return View{}, err
	}
	court, ok := v.court(req.CourtID)
	if !ok {
		return View{}, fmt.Errorf("%w: no such court", ErrNotFound)
	}
	end := req.Start.Add(time.Duration(req.Hours) * time.Hour)
	if req.Start.In(v.location).Minute() != 0 {
		return View{}, fmt.Errorf("%w: players book from the start of an hour", ErrInvalidRequest)
	}
	if err := v.checkPeriod(req.Start, end); err != nil {
		return View{}, err
	}
	if !req.Start.After(s.Now()) {
		return View{}, ErrInThePast
	}
	price, err := Price(v.rules, court.Sport, req.Start.In(v.location), end.In(v.location))
	if err != nil {
		return View{}, err
	}
	held, err := PlaceHold(ctx, s.Pool, HoldRequest{
		CourtID:       court.ID,
		Start:         req.Start,
		End:           end,
		CustomerName:  req.CustomerName,
		CustomerPhone: phone,
		PriceIDR:      price,
		DepositIDR:    Deposit(price),
	})
	if err != nil {
		return View{}, err
	}
	return View{Booking: held, CourtName: court.Name, Sport: court.Sport}, nil
}

// ByReference returns a booking for the player's confirmation screen.
func (s *Service) ByReference(ctx context.Context, reference string) (View, error) {
	row, err := store.New(s.Pool).GetBookingByReference(ctx, reference)
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, fmt.Errorf("get booking: %w", err)
	}
	return View{Booking: row.Booking, CourtName: row.CourtName, Sport: row.Sport}, nil
}

// OwnerDay returns every court and every booking the board shows for one date.
func (s *Service) OwnerDay(ctx context.Context, date time.Time) ([]Court, []View, error) {
	q := store.New(s.Pool)
	v, err := s.loadVenue(ctx, q)
	if err != nil {
		return nil, nil, err
	}
	dayStart, dayEnd := v.dayBounds(date)
	rows, err := q.ListDayBookingsForOwner(ctx, store.ListDayBookingsForOwnerParams{
		FromTime: pgtype.Timestamptz{Time: dayStart, Valid: true},
		ToTime:   pgtype.Timestamptz{Time: dayEnd, Valid: true},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("load day bookings: %w", err)
	}
	views := make([]View, 0, len(rows))
	for _, row := range rows {
		views = append(views, View{Booking: row.Booking, CourtName: row.CourtName, Sport: row.Sport})
	}
	return v.courts, views, nil
}

// WalkIn is a booking the owner makes for someone at the desk.
type WalkIn struct {
	CourtID       pgtype.UUID
	Start, End    time.Time
	CustomerName  string
	CustomerPhone string // optional
}

// CreateWalkIn books a court for a desk customer at the listed price, with no deposit.
func (s *Service) CreateWalkIn(ctx context.Context, req WalkIn) (View, error) {
	phone := ""
	if req.CustomerPhone != "" {
		normalized, err := NormalizePhone(req.CustomerPhone)
		if err != nil {
			return View{}, err
		}
		phone = normalized
	}
	q := store.New(s.Pool)
	v, err := s.loadVenue(ctx, q)
	if err != nil {
		return View{}, err
	}
	court, ok := v.court(req.CourtID)
	if !ok {
		return View{}, fmt.Errorf("%w: no such court", ErrNotFound)
	}
	if err := v.checkPeriod(req.Start, req.End); err != nil {
		return View{}, err
	}
	// A walk-in may start in the half hour that is already running, but not end in the past.
	if !req.End.After(s.Now()) {
		return View{}, ErrInThePast
	}
	price, err := Price(v.rules, court.Sport, req.Start.In(v.location), req.End.In(v.location))
	if err != nil {
		return View{}, err
	}
	reference, err := NewReference()
	if err != nil {
		return View{}, err
	}
	var created store.Booking
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		txq := store.New(tx)
		if err := txq.LockCourt(ctx, court.ID); err != nil {
			return fmt.Errorf("lock court: %w", err)
		}
		period := Period(req.Start, req.End)
		if _, err := txq.ExpireOverlappingHolds(ctx, store.ExpireOverlappingHoldsParams{
			CourtID: court.ID, Period: period,
		}); err != nil {
			return fmt.Errorf("expire stale holds: %w", err)
		}
		created, err = txq.InsertWalkIn(ctx, store.InsertWalkInParams{
			Reference:     reference,
			CourtID:       court.ID,
			Period:        period,
			CustomerName:  req.CustomerName,
			CustomerPhone: phone,
			PriceIdr:      price,
		})
		return err
	})
	if isOverlapViolation(err) {
		return View{}, ErrSlotTaken
	}
	if err != nil {
		return View{}, fmt.Errorf("create walk-in: %w", err)
	}
	return View{Booking: created, CourtName: court.Name, Sport: court.Sport}, nil
}

// Reschedule moves a confirmed booking or a walk-in to another court of the same sport or
// another time. The price stays as agreed; the exclusion constraint refuses a conflict.
func (s *Service) Reschedule(ctx context.Context, id, courtID pgtype.UUID, start, end time.Time) (View, error) {
	q := store.New(s.Pool)
	v, err := s.loadVenue(ctx, q)
	if err != nil {
		return View{}, err
	}
	current, err := q.GetBookingByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, fmt.Errorf("get booking: %w", err)
	}
	court, ok := v.court(courtID)
	if !ok {
		return View{}, fmt.Errorf("%w: no such court", ErrNotFound)
	}
	if court.Sport != current.Sport {
		return View{}, fmt.Errorf("%w: a %s booking cannot move to a %s court", ErrInvalidRequest,
			current.Sport, court.Sport)
	}
	if err := v.checkPeriod(start, end); err != nil {
		return View{}, err
	}
	if !end.After(s.Now()) {
		return View{}, ErrInThePast
	}
	var moved store.Booking
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		txq := store.New(tx)
		if err := txq.LockCourt(ctx, court.ID); err != nil {
			return fmt.Errorf("lock court: %w", err)
		}
		var rescheduleErr error
		moved, rescheduleErr = txq.Reschedule(ctx, store.RescheduleParams{
			ID: id, CourtID: court.ID, Period: Period(start, end),
		})
		return rescheduleErr
	})
	if isOverlapViolation(err) {
		return View{}, ErrSlotTaken
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, fmt.Errorf("%w: only confirmed bookings and walk-ins can be moved",
			ErrInvalidTransition)
	}
	if err != nil {
		return View{}, fmt.Errorf("reschedule: %w", err)
	}
	return View{Booking: moved, CourtName: court.Name, Sport: court.Sport}, nil
}

// allowedFrom lists, for each status the owner can set, the statuses it can come from.
var allowedFrom = map[store.BookingStatus][]store.BookingStatus{
	store.BookingStatusConfirmed: {store.BookingStatusHeld}, // paid at the desk
	store.BookingStatusCancelled: {store.BookingStatusHeld, store.BookingStatusConfirmed,
		store.BookingStatusWalkIn},
	store.BookingStatusNoShow: {store.BookingStatusConfirmed, store.BookingStatusWalkIn},
}

// ChangeStatus confirms a held booking paid at the desk, cancels a booking, or marks a no-show.
// A no-show can only be marked once the booking has started.
func (s *Service) ChangeStatus(ctx context.Context, id pgtype.UUID, to store.BookingStatus) (View, error) {
	from, ok := allowedFrom[to]
	if !ok {
		return View{}, fmt.Errorf("%w: the owner cannot set status %q", ErrInvalidRequest, to)
	}
	q := store.New(s.Pool)
	current, err := q.GetBookingByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, fmt.Errorf("get booking: %w", err)
	}
	if to == store.BookingStatusNoShow && s.Now().Before(current.Booking.Period.Lower.Time) {
		return View{}, fmt.Errorf("%w: a no-show can be marked once the booking has started",
			ErrInvalidTransition)
	}
	fromText := make([]string, len(from))
	for i, status := range from {
		fromText[i] = string(status)
	}
	// Confirming a hold at the desk puts it back under the constraint, so lock the court first.
	var changed store.Booking
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		txq := store.New(tx)
		if err := txq.LockCourtOfBooking(ctx, id); err != nil {
			return fmt.Errorf("lock court: %w", err)
		}
		var changeErr error
		changed, changeErr = txq.ChangeStatus(ctx, store.ChangeStatusParams{
			ID: id, ToStatus: to, AllowedFrom: fromText,
		})
		return changeErr
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, fmt.Errorf("%w: a %s booking cannot become %s", ErrInvalidTransition,
			current.Booking.Status, to)
	}
	if err != nil {
		return View{}, fmt.Errorf("change status: %w", err)
	}
	return View{Booking: changed, CourtName: current.CourtName, Sport: current.Sport}, nil
}

// ExpireHolds marks every hold whose time is over as expired and reports how many.
func (s *Service) ExpireHolds(ctx context.Context) (int64, error) {
	expired, err := store.New(s.Pool).ExpireHolds(ctx)
	if err != nil {
		return 0, fmt.Errorf("expire holds: %w", err)
	}
	return expired, nil
}
