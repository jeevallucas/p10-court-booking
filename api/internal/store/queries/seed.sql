-- name: ResetDemoBookings :exec
-- The nightly reseed starts from a clean schedule; notifications and messages refer to bookings.
truncate payment_notification, outbox_message, booking;

-- name: FindVenueByName :one
select id from venue where name = sqlc.arg(name);

-- name: InsertVenue :one
insert into venue (name, timezone, opens_at, closes_at)
values (sqlc.arg(name), 'Asia/Jakarta', '07:00', '23:00')
returning id;

-- name: InsertCourt :exec
insert into court (venue_id, name, sport, sort_order)
values (sqlc.arg(venue_id), sqlc.arg(name), sqlc.arg(sport), sqlc.arg(sort_order));

-- name: InsertPriceRule :exec
insert into price_rule (venue_id, sport, starts_at, ends_at, hourly_price_idr)
values (sqlc.arg(venue_id), sqlc.arg(sport), sqlc.arg(starts_at), sqlc.arg(ends_at),
    sqlc.arg(hourly_price_idr));

-- name: UpsertOwner :exec
insert into owner_account (venue_id, email, password_hash, display_name)
values (sqlc.arg(venue_id), sqlc.arg(email), sqlc.arg(password_hash), sqlc.arg(display_name))
on conflict (email) do update set password_hash = excluded.password_hash,
    display_name = excluded.display_name;

-- name: InsertSeedBooking :exec
insert into booking (
    reference, court_id, period, status, customer_name, customer_phone, price_idr, deposit_idr,
    created_at
) values (
    sqlc.arg(reference), sqlc.arg(court_id), sqlc.arg(period)::tstzrange, sqlc.arg(status),
    sqlc.arg(customer_name), sqlc.arg(customer_phone), sqlc.arg(price_idr),
    sqlc.arg(deposit_idr), sqlc.arg(created_at)
);
