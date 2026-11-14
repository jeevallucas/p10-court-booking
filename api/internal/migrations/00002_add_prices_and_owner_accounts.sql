-- +goose Up
alter table venue
    add column opens_at time not null default '07:00',
    add column closes_at time not null default '23:00';

-- Hourly price by sport and time of day, in the venue's local time. Bookings are priced per
-- half hour, so a booking crossing 17.00 pays each part at its own rate.
create table price_rule (
    id uuid primary key default gen_random_uuid(),
    venue_id uuid not null references venue (id),
    sport text not null check (sport in ('badminton', 'futsal')),
    starts_at time not null,
    ends_at time not null,
    hourly_price_idr integer not null check (hourly_price_idr > 0),
    check (starts_at < ends_at),
    constraint price_rule_no_overlap exclude using gist (
        venue_id with =, sport with =, tsrange('2000-01-01'::date + starts_at,
            '2000-01-01'::date + ends_at) with &&
    )
);

create table owner_account (
    id uuid primary key default gen_random_uuid(),
    venue_id uuid not null references venue (id),
    email text not null unique,
    password_hash text not null,
    display_name text not null
);

-- Only a SHA-256 hash of the session token is stored, so a database leak does not hand out
-- live sessions.
create table owner_session (
    token_hash bytea primary key,
    owner_id uuid not null references owner_account (id) on delete cascade,
    expires_at timestamptz not null,
    created_at timestamptz not null default now()
);

create index owner_session_expires_idx on owner_session (expires_at);

-- +goose Down
drop table owner_session;
drop table owner_account;
drop table price_rule;
alter table venue drop column closes_at, drop column opens_at;
