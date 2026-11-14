-- name: LockCourt :exec
-- Every write that puts a booking on a court takes this lock first, so writers for one court
-- queue. The exclusion constraint alone keeps bookings apart, but two transactions inserting
-- overlapping rows at the same moment each wait for the other's uncommitted row and deadlock;
-- the load test hit this (docs/load-test.md). Taking the court before any booking row also
-- gives every writer the same lock order.
select id from court where id = sqlc.arg(id) for no key update;

-- name: LockCourtOfBooking :exec
select id from court
where id = (select court_id from booking where booking.id = sqlc.arg(booking_id))
for no key update;

-- name: ExpireOverlappingHolds :execrows
-- Marks holds whose time has run out on this court and period as expired, so the exclusion
-- constraint stops counting them before a new booking is inserted.
update booking
set status = 'expired', hold_expires_at = null, updated_at = now()
where court_id = sqlc.arg(court_id)
    and period && sqlc.arg(period)::tstzrange
    and status = 'held'
    and hold_expires_at <= now();

-- name: InsertHold :one
-- The hold's expiry comes from the database clock, the same clock ExpireOverlappingHolds reads.
insert into booking (
    reference, court_id, period, status, customer_name, customer_phone, price_idr, deposit_idr,
    hold_expires_at
) values (
    sqlc.arg(reference), sqlc.arg(court_id), sqlc.arg(period)::tstzrange, 'held',
    sqlc.arg(customer_name), sqlc.arg(customer_phone), sqlc.arg(price_idr), sqlc.arg(deposit_idr),
    now() + sqlc.arg(hold_for)::interval
)
returning *;

-- name: InsertWalkIn :one
insert into booking (
    reference, court_id, period, status, customer_name, customer_phone, price_idr, deposit_idr
) values (
    sqlc.arg(reference), sqlc.arg(court_id), sqlc.arg(period)::tstzrange, 'walk_in',
    sqlc.arg(customer_name), sqlc.arg(customer_phone), sqlc.arg(price_idr), 0
)
returning *;

-- name: ExpireHolds :execrows
-- The periodic sweep: frees every hold whose ten minutes are over. It skips holds another
-- transaction has locked rather than wait inside a booking write; the next sweep gets them.
update booking
set status = 'expired', hold_expires_at = null, updated_at = now()
where id in (
    select id from booking
    where status = 'held' and hold_expires_at <= now()
    for update skip locked
);

-- name: GetBookingByReference :one
select sqlc.embed(booking), court.name as court_name, court.sport
from booking
join court on court.id = booking.court_id
where booking.reference = sqlc.arg(reference);

-- name: GetBookingByID :one
select sqlc.embed(booking), court.name as court_name, court.sport
from booking
join court on court.id = booking.court_id
where booking.id = sqlc.arg(id);

-- name: ListOccupyingBookings :many
-- Bookings that hold a court during [from, to): confirmed, walk-in, and holds still running.
select sqlc.embed(booking), court.name as court_name, court.sport
from booking
join court on court.id = booking.court_id
where booking.period && tstzrange(sqlc.arg(from_time)::timestamptz, sqlc.arg(to_time)::timestamptz, '[)')
    and (booking.status in ('confirmed', 'walk_in')
        or (booking.status = 'held' and booking.hold_expires_at > now()))
order by court.sort_order, lower(booking.period);

-- name: ListDayBookingsForOwner :many
-- The board also shows no-shows, so the owner sees what happened to the slot.
select sqlc.embed(booking), court.name as court_name, court.sport
from booking
join court on court.id = booking.court_id
where booking.period && tstzrange(sqlc.arg(from_time)::timestamptz, sqlc.arg(to_time)::timestamptz, '[)')
    and (booking.status in ('confirmed', 'walk_in', 'no_show')
        or (booking.status = 'held' and booking.hold_expires_at > now()))
order by court.sort_order, lower(booking.period);

-- name: ChangeStatus :one
-- Applies a status change only from an allowed current status; no row means it was not allowed
-- (or the booking does not exist).
update booking
set status = sqlc.arg(to_status), hold_expires_at = null, updated_at = now()
where id = sqlc.arg(id) and status::text = any(sqlc.arg(allowed_from)::text[])
returning *;

-- name: Reschedule :one
update booking
set court_id = sqlc.arg(court_id), period = sqlc.arg(period)::tstzrange, updated_at = now()
where id = sqlc.arg(id) and status in ('confirmed', 'walk_in')
returning *;
