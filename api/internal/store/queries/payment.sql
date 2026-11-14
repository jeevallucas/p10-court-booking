-- name: LockBookingByReference :one
-- Locks the booking so concurrent notifications for it are applied one at a time.
select sqlc.embed(booking), court.name as court_name, court.sport
from booking
join court on court.id = booking.court_id
where booking.reference = sqlc.arg(reference)
for update of booking;

-- name: InsertNotification :one
-- Returns no row when this transaction and status were already recorded: a replay.
insert into payment_notification (
    booking_id, transaction_id, transaction_status, gross_amount_idr, outcome, payload
) values (
    sqlc.arg(booking_id), sqlc.arg(transaction_id), sqlc.arg(transaction_status),
    sqlc.arg(gross_amount_idr), sqlc.arg(outcome), sqlc.arg(payload)
)
on conflict (transaction_id, transaction_status) do nothing
returning id;

-- name: ConfirmPaidBooking :one
-- A paid hold becomes confirmed. An expired hold is claimed back if the slot is still free;
-- the exclusion constraint refuses it if someone else has booked the time since.
update booking
set status = 'confirmed', hold_expires_at = null, updated_at = now()
where id = sqlc.arg(id) and status in ('held', 'expired')
returning *;

-- name: ReleaseHold :one
update booking
set status = 'cancelled', hold_expires_at = null, updated_at = now()
where id = sqlc.arg(id) and status = 'held'
returning *;
