package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/owner"
	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

// SessionCookie is the cookie that carries the owner's session token.
const SessionCookie = "court_session"

// server implements the generated StrictServerInterface on top of the booking rules.
type server struct {
	bookings      *booking.Service
	sessions      *owner.Sessions
	secureCookies bool
}

var _ StrictServerInterface = (*server)(nil)

func problem(code ProblemCode, detail string) ProblemJSONResponse {
	return ProblemJSONResponse{Code: code, Detail: detail}
}

// problemFor names the reason a booking rule refused a request. ok is false for unexpected
// errors, which become a logged 500.
func problemFor(err error) (ProblemJSONResponse, bool) {
	switch {
	case errors.Is(err, booking.ErrSlotTaken):
		return problem(SlotTaken, err.Error()), true
	case errors.Is(err, booking.ErrInvalidTransition):
		return problem(InvalidTransition, err.Error()), true
	case errors.Is(err, booking.ErrOutsideOpeningHours):
		return problem(OutsideOpeningHours, err.Error()), true
	case errors.Is(err, booking.ErrInThePast):
		return problem(InThePast, err.Error()), true
	case errors.Is(err, booking.ErrInvalidRequest), errors.Is(err, booking.ErrInvalidPeriod):
		return problem(InvalidRequest, err.Error()), true
	case errors.Is(err, booking.ErrNotFound):
		return problem(NotFound, err.Error()), true
	}
	return ProblemJSONResponse{}, false
}

func (s *server) GetAvailability(ctx context.Context, req GetAvailabilityRequestObject) (GetAvailabilityResponseObject, error) {
	courts, slots, err := s.bookings.Availability(ctx, req.Params.Date.Time, string(req.Params.Sport))
	if err != nil {
		return nil, err
	}
	response := GetAvailability200JSONResponse{
		Date:   req.Params.Date,
		Sport:  req.Params.Sport,
		Courts: make([]Court, 0, len(courts)),
		Slots:  make([]Slot, 0, len(slots)),
	}
	for _, c := range courts {
		response.Courts = append(response.Courts, courtJSON(c))
	}
	for _, slot := range slots {
		response.Slots = append(response.Slots, Slot{
			CourtId:   uuid.UUID(slot.CourtID.Bytes),
			Start:     slot.Start,
			End:       slot.End,
			PriceIdr:  int(slot.PriceIDR),
			Available: slot.Available,
		})
	}
	return response, nil
}

func (s *server) PlaceHold(ctx context.Context, req PlaceHoldRequestObject) (PlaceHoldResponseObject, error) {
	held, err := s.bookings.HoldForPlayer(ctx, booking.PlayerHold{
		CourtID:       pgUUID(req.Body.CourtId),
		Start:         req.Body.Start,
		Hours:         req.Body.Hours,
		CustomerName:  req.Body.CustomerName,
		CustomerPhone: req.Body.CustomerPhone,
	})
	if err == nil {
		return PlaceHold201JSONResponse(bookingJSON(held)), nil
	}
	p, ok := problemFor(err)
	switch {
	case !ok:
		return nil, err
	case p.Code == SlotTaken:
		return PlaceHold409JSONResponse{p}, nil
	default:
		return PlaceHold422JSONResponse(p), nil
	}
}

func (s *server) GetBooking(ctx context.Context, req GetBookingRequestObject) (GetBookingResponseObject, error) {
	view, err := s.bookings.ByReference(ctx, req.Reference)
	if errors.Is(err, booking.ErrNotFound) {
		return GetBooking404JSONResponse{problem(NotFound, "no booking has this reference")}, nil
	}
	if err != nil {
		return nil, err
	}
	return GetBooking200JSONResponse(bookingJSON(view)), nil
}

func (s *server) LogIn(ctx context.Context, req LogInRequestObject) (LogInResponseObject, error) {
	token, err := s.sessions.LogIn(ctx, string(req.Body.Email), req.Body.Password)
	if errors.Is(err, owner.ErrInvalidCredentials) {
		return LogIn401JSONResponse{problem(Unauthorized, err.Error())}, nil
	}
	if err != nil {
		return nil, err
	}
	cookie := s.cookie(token, int(s.sessions.Lifetime.Seconds()))
	return LogIn204Response{Headers: LogIn204ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s *server) LogOut(ctx context.Context, _ LogOutRequestObject) (LogOutResponseObject, error) {
	if err := s.sessions.LogOut(ctx, sessionToken(ctx)); err != nil {
		return nil, err
	}
	cookie := s.cookie("", -1)
	return LogOut204Response{Headers: LogOut204ResponseHeaders{SetCookie: &cookie}}, nil
}

// cookie keeps the session first-party: the web app proxies /v1 to the API, so SameSite=Lax
// suffices and no cross-site cookie is needed.
func (s *server) cookie(token string, maxAge int) string {
	c := http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	}
	return c.String()
}

func (s *server) ListOwnerBookings(ctx context.Context, req ListOwnerBookingsRequestObject) (ListOwnerBookingsResponseObject, error) {
	courts, views, err := s.bookings.OwnerDay(ctx, req.Params.Date.Time)
	if err != nil {
		return nil, err
	}
	response := ListOwnerBookings200JSONResponse{
		Date:     req.Params.Date,
		Courts:   make([]Court, 0, len(courts)),
		Bookings: make([]OwnerBooking, 0, len(views)),
	}
	for _, c := range courts {
		response.Courts = append(response.Courts, courtJSON(c))
	}
	for _, v := range views {
		response.Bookings = append(response.Bookings, ownerBookingJSON(v))
	}
	return response, nil
}

func (s *server) CreateWalkIn(ctx context.Context, req CreateWalkInRequestObject) (CreateWalkInResponseObject, error) {
	phone := ""
	if req.Body.CustomerPhone != nil {
		phone = *req.Body.CustomerPhone
	}
	created, err := s.bookings.CreateWalkIn(ctx, booking.WalkIn{
		CourtID:       pgUUID(req.Body.CourtId),
		Start:         req.Body.Start,
		End:           req.Body.End,
		CustomerName:  req.Body.CustomerName,
		CustomerPhone: phone,
	})
	if err == nil {
		return CreateWalkIn201JSONResponse(ownerBookingJSON(created)), nil
	}
	p, ok := problemFor(err)
	switch {
	case !ok:
		return nil, err
	case p.Code == SlotTaken:
		return CreateWalkIn409JSONResponse(p), nil
	default:
		return CreateWalkIn422JSONResponse(p), nil
	}
}

func (s *server) RescheduleBooking(ctx context.Context, req RescheduleBookingRequestObject) (RescheduleBookingResponseObject, error) {
	moved, err := s.bookings.Reschedule(ctx, pgUUID(req.Id), pgUUID(req.Body.CourtId),
		req.Body.Start, req.Body.End)
	if err == nil {
		return RescheduleBooking200JSONResponse(ownerBookingJSON(moved)), nil
	}
	p, ok := problemFor(err)
	switch {
	case !ok:
		return nil, err
	case p.Code == NotFound:
		return RescheduleBooking404JSONResponse(p), nil
	case p.Code == SlotTaken, p.Code == InvalidTransition:
		return RescheduleBooking409JSONResponse(p), nil
	default:
		return RescheduleBooking422JSONResponse(p), nil
	}
}

func (s *server) ChangeBookingStatus(ctx context.Context, req ChangeBookingStatusRequestObject) (ChangeBookingStatusResponseObject, error) {
	changed, err := s.bookings.ChangeStatus(ctx, pgUUID(req.Id), store.BookingStatus(req.Body.Status))
	if err == nil {
		return ChangeBookingStatus200JSONResponse(ownerBookingJSON(changed)), nil
	}
	p, ok := problemFor(err)
	switch {
	case !ok:
		return nil, err
	case p.Code == NotFound:
		return ChangeBookingStatus404JSONResponse(p), nil
	default:
		return ChangeBookingStatus409JSONResponse(p), nil
	}
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func courtJSON(c booking.Court) Court {
	return Court{Id: uuid.UUID(c.ID.Bytes), Name: c.Name, Sport: Sport(c.Sport)}
}

func bookingJSON(v booking.View) Booking {
	b := Booking{
		Reference:  v.Reference,
		CourtName:  v.CourtName,
		Sport:      Sport(v.Sport),
		Start:      v.Period.Lower.Time,
		End:        v.Period.Upper.Time,
		Status:     BookingStatus(v.Status),
		PriceIdr:   int(v.PriceIdr),
		DepositIdr: int(v.DepositIdr),
	}
	if v.HoldExpiresAt.Valid {
		expires := v.HoldExpiresAt.Time.UTC().Truncate(time.Second)
		b.HoldExpiresAt = &expires
	}
	return b
}

func ownerBookingJSON(v booking.View) OwnerBooking {
	b := bookingJSON(v)
	return OwnerBooking{
		Id:            uuid.UUID(v.ID.Bytes),
		CourtId:       uuid.UUID(v.CourtID.Bytes),
		CustomerName:  v.CustomerName,
		CustomerPhone: v.CustomerPhone,
		Reference:     b.Reference,
		CourtName:     b.CourtName,
		Sport:         b.Sport,
		Start:         b.Start,
		End:           b.End,
		Status:        b.Status,
		PriceIdr:      b.PriceIdr,
		DepositIdr:    b.DepositIdr,
		HoldExpiresAt: b.HoldExpiresAt,
	}
}
