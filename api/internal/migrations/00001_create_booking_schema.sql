-- +goose Up
-- btree_gist lets the exclusion constraint compare court_id (a scalar) with = inside a GiST index.
create extension if not exists btree_gist;

create table venue (
    id uuid primary key default gen_random_uuid(),
    name text not null,
    timezone text not null default 'Asia/Jakarta'
);

create table court (
    id uuid primary key default gen_random_uuid(),
    venue_id uuid not null references venue (id),
    name text not null,
    sport text not null check (sport in ('badminton', 'futsal')),
    sort_order integer not null,
    unique (venue_id, name)
);

create type booking_status as enum (
    'held', 'confirmed', 'walk_in', 'cancelled', 'expired', 'no_show'
);

create table booking (
    id uuid primary key default gen_random_uuid(),
    reference text not null unique,
    court_id uuid not null references court (id),
    -- Half-open range [start, end): a booking ending at 20.00 and one starting at 20.00 do not
    -- overlap.
    period tstzrange not null check (lower(period) < upper(period) and lower_inc(period)
        and not upper_inc(period)),
    status booking_status not null,
    customer_name text not null,
    customer_phone text not null,
    price_idr integer not null check (price_idr >= 0),
    deposit_idr integer not null check (deposit_idr >= 0 and deposit_idr <= price_idr),
    hold_expires_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    check ((status = 'held') = (hold_expires_at is not null)),
    -- The guarantee: no two bookings that still occupy a court may overlap in time. A hold whose
    -- time has passed keeps its row until it is marked expired, which hold creation does first
    -- in the same transaction.
    constraint booking_no_overlap exclude using gist (court_id with =, period with &&)
        where (status in ('held', 'confirmed', 'walk_in'))
);

create index booking_court_period_idx on booking using gist (court_id, period);

-- +goose Down
drop table booking;
drop type booking_status;
drop table court;
drop table venue;
