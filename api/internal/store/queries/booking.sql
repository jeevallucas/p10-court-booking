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
