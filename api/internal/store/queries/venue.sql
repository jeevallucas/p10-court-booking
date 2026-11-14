-- name: GetCourt :one
select court.id, court.name, court.sport, court.sort_order, venue.id as venue_id,
    venue.timezone, venue.opens_at, venue.closes_at
from court
join venue on venue.id = court.venue_id
where court.id = sqlc.arg(id);

-- name: GetVenue :one
-- One venue per deployment; a multi-venue marketplace is a non-goal.
select id, name, timezone, opens_at, closes_at
from venue
order by name
limit 1;

-- name: ListCourts :many
select id, name, sport, sort_order
from court
where venue_id = sqlc.arg(venue_id)
order by sort_order;

-- name: ListPriceRules :many
select sport, starts_at, ends_at, hourly_price_idr
from price_rule
where venue_id = sqlc.arg(venue_id)
order by sport, starts_at;
