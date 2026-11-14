-- name: GetOwnerByEmail :one
select id, venue_id, email, password_hash, display_name
from owner_account
where email = lower(sqlc.arg(email));

-- name: InsertSession :exec
insert into owner_session (token_hash, owner_id, expires_at)
values (sqlc.arg(token_hash), sqlc.arg(owner_id), sqlc.arg(expires_at));

-- name: GetSessionOwner :one
select owner_account.id, owner_account.venue_id, owner_account.display_name
from owner_session
join owner_account on owner_account.id = owner_session.owner_id
where owner_session.token_hash = sqlc.arg(token_hash) and owner_session.expires_at > now();

-- name: DeleteSession :exec
delete from owner_session where token_hash = sqlc.arg(token_hash);

-- name: DeleteExpiredSessions :execrows
delete from owner_session where expires_at <= now();
