-- sqlc query definitions for the Budget Tracker transactions table.

-- name: CreateTransaction :one
INSERT INTO transactions (type, amount_cents, date, category, description)
VALUES (?, ?, ?, ?, ?)
RETURNING id, type, amount_cents, date, category, description;

-- name: GetTransaction :one
SELECT id, type, amount_cents, date, category, description
FROM transactions
WHERE id = ?;

-- name: UpdateTransaction :one
UPDATE transactions
SET type = ?, amount_cents = ?, date = ?, category = ?, description = ?
WHERE id = ?
RETURNING id, type, amount_cents, date, category, description;

-- name: DeleteTransaction :exec
DELETE FROM transactions
WHERE id = ?;

-- name: ListTransactions :many
SELECT id, type, amount_cents, date, category, description
FROM transactions
ORDER BY date DESC, id ASC;
