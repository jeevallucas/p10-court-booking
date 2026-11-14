-- +goose Up
-- Every payment notification that passed the signature check, kept for audit. The unique key
-- makes a replayed notification a no-op: the provider may send the same one many times.
create table payment_notification (
    id bigint generated always as identity primary key,
    booking_id uuid not null references booking (id),
    transaction_id text not null,
    transaction_status text not null,
    gross_amount_idr integer not null,
    outcome text not null check (outcome in (
        'confirmed', 'released', 'ignored', 'refund_needed'
    )),
    payload jsonb not null,
    received_at timestamptz not null default now(),
    unique (transaction_id, transaction_status)
);

-- Messages to send after a commit, written in the same transaction as the change they report,
-- so a confirmation is never sent for a change that rolled back and never lost for one that
-- committed.
create table outbox_message (
    id bigint generated always as identity primary key,
    recipient text not null,
    subject text not null,
    body text not null,
    created_at timestamptz not null default now(),
    next_attempt_at timestamptz not null default now(),
    attempts integer not null default 0,
    last_error text,
    sent_at timestamptz
);

create index outbox_message_due_idx on outbox_message (next_attempt_at) where sent_at is null;

-- +goose Down
drop table outbox_message;
drop table payment_notification;
