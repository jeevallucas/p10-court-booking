-- name: EnqueueMessage :exec
insert into outbox_message (recipient, subject, body)
values (sqlc.arg(recipient), sqlc.arg(subject), sqlc.arg(body));

-- name: ClaimDueMessages :many
-- SKIP LOCKED lets several workers run without two of them sending the same message.
select id, recipient, subject, body, attempts
from outbox_message
where sent_at is null and next_attempt_at <= now()
order by id
limit sqlc.arg(batch_size)
for update skip locked;

-- name: MarkMessageSent :exec
update outbox_message set sent_at = now(), last_error = null where id = sqlc.arg(id);

-- name: MarkMessageFailed :exec
update outbox_message
set attempts = attempts + 1, last_error = sqlc.arg(last_error),
    next_attempt_at = now() + sqlc.arg(retry_in)::interval
where id = sqlc.arg(id);
