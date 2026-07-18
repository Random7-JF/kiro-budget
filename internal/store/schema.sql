-- Schema for the Budget Tracker SQLite data store.
--
-- Amounts are stored as a positive integer count of cents; the transaction
-- type distinguishes income from expense. Dates are ISO 8601 (YYYY-MM-DD)
-- strings, which sort lexicographically in calendar order. All financial data
-- is scoped to a user via user_id.
--
-- This file creates tables only (CREATE TABLE IF NOT EXISTS). User-scoped
-- indexes are created after migrations run, so an older database can gain its
-- user_id column before an index that references it is created.

CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT    NOT NULL,
    first_name    TEXT    NOT NULL DEFAULT '',
    last_name     TEXT    NOT NULL DEFAULT '',
    email         TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT    NOT NULL PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS transactions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL,
    type         TEXT    NOT NULL CHECK (type IN ('expense', 'income')),
    amount_cents INTEGER NOT NULL CHECK (amount_cents > 0 AND amount_cents <= 99999999999),
    date         TEXT    NOT NULL, -- ISO 8601 YYYY-MM-DD
    category     TEXT    NOT NULL CHECK (length(category) BETWEEN 1 AND 100),
    description  TEXT    NOT NULL DEFAULT ''
);

-- Per-category monthly spending limits, scoped per user.
CREATE TABLE IF NOT EXISTS budgets (
    user_id      INTEGER NOT NULL,
    category     TEXT    NOT NULL CHECK (length(category) BETWEEN 1 AND 100),
    amount_cents INTEGER NOT NULL CHECK (amount_cents > 0 AND amount_cents <= 99999999999),
    PRIMARY KEY (user_id, category)
);

-- Recurring transaction templates that repeat monthly on a day of the month.
CREATE TABLE IF NOT EXISTS recurring (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL,
    type         TEXT    NOT NULL CHECK (type IN ('expense', 'income')),
    amount_cents INTEGER NOT NULL CHECK (amount_cents > 0 AND amount_cents <= 99999999999),
    category     TEXT    NOT NULL CHECK (length(category) BETWEEN 1 AND 100),
    description  TEXT    NOT NULL DEFAULT '',
    day_of_month INTEGER NOT NULL CHECK (day_of_month BETWEEN 1 AND 31)
);

-- Idempotency ledger: records which recurring rule was posted for which month
-- (YYYY-MM) so re-posting a month never creates duplicates.
CREATE TABLE IF NOT EXISTS recurring_postings (
    recurring_id INTEGER NOT NULL,
    ym           TEXT    NOT NULL,
    PRIMARY KEY (recurring_id, ym)
);

-- Bills: recurring obligations to a payee at a given frequency, scoped per user.
CREATE TABLE IF NOT EXISTS bills (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL,
    payee        TEXT    NOT NULL CHECK (length(payee) BETWEEN 1 AND 100),
    amount_cents INTEGER NOT NULL CHECK (amount_cents > 0 AND amount_cents <= 99999999999),
    frequency    TEXT    NOT NULL CHECK (frequency IN ('weekly', 'biweekly', 'monthly', 'quarterly', 'yearly')),
    category     TEXT    NOT NULL DEFAULT '',
    due_day      INTEGER NOT NULL DEFAULT 0 CHECK (due_day BETWEEN 0 AND 31)
);
