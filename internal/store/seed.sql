-- Seed data for the Budget Tracker: a coherent demo dataset for one person
-- covering May–July 2026, plus budgets, recurring rules, and bills.
--
-- This script is idempotent: it clears all data first (and resets the
-- AUTOINCREMENT counters) before inserting, so applying it always yields the
-- same known state. It is executed by `Repo.Seed` and the `budgetctl seed`
-- command. All amounts are integer cents.

DELETE FROM recurring_postings;
DELETE FROM transactions;
DELETE FROM budgets;
DELETE FROM recurring;
DELETE FROM bills;
DELETE FROM sqlite_sequence;

-- Transactions -------------------------------------------------------------
INSERT INTO transactions (type, amount_cents, date, category, description) VALUES
  -- May 2026
  ('income',  230000, '2026-05-01', 'Salary',         'Biweekly paycheck'),
  ('income',  230000, '2026-05-15', 'Salary',         'Biweekly paycheck'),
  ('expense', 135000, '2026-05-01', 'Rent',           'Monthly rent'),
  ('expense',   6999, '2026-05-02', 'Utilities',      'Internet service'),
  ('expense',   8240, '2026-05-05', 'Groceries',      'Grocery run'),
  ('expense',   2460, '2026-05-09', 'Dining',         'Dinner out'),
  ('expense',   4500, '2026-05-12', 'Transportation', 'Gas fill-up'),
  ('expense',   1599, '2026-05-15', 'Subscriptions',  'Netflix'),
  ('expense',   6125, '2026-05-18', 'Groceries',      'Groceries'),
  ('expense',   7834, '2026-05-20', 'Utilities',      'Electricity bill'),
  ('expense',   3000, '2026-05-24', 'Entertainment',  'Concert ticket'),
  ('expense',   2500, '2026-05-28', 'Health',         'Pharmacy'),
  -- June 2026
  ('income',  230000, '2026-06-12', 'Salary',         'Biweekly paycheck'),
  ('income',  230000, '2026-06-26', 'Salary',         'Biweekly paycheck'),
  ('income',   45000, '2026-06-28', 'Freelance',      'Freelance project'),
  ('expense', 135000, '2026-06-01', 'Rent',           'Monthly rent'),
  ('expense',   6999, '2026-06-02', 'Utilities',      'Internet service'),
  ('expense',   7410, '2026-06-05', 'Groceries',      'Groceries'),
  ('expense',   1875, '2026-06-10', 'Dining',         'Lunch with coworker'),
  ('expense',   4475, '2026-06-15', 'Transportation', 'Gas fill-up'),
  ('expense',   1599, '2026-06-19', 'Subscriptions',  'Netflix'),
  ('expense',   7834, '2026-06-20', 'Utilities',      'Electricity bill'),
  ('expense',   8999, '2026-06-22', 'Shopping',       'New shoes'),
  ('expense',   3280, '2026-06-25', 'Dining',         'Dinner out'),
  ('expense',   4500, '2026-06-29', 'Utilities',      'Water bill'),
  -- July 2026
  ('income',  230000, '2026-07-10', 'Salary',         'Biweekly paycheck'),
  ('income',    1235, '2026-07-10', 'Interest',       'Savings account interest'),
  ('expense', 135000, '2026-07-01', 'Rent',           'Monthly rent'),
  ('expense',   6999, '2026-07-02', 'Utilities',      'Internet service'),
  ('expense',   5500, '2026-07-02', 'Phone',          'Cell phone bill'),
  ('expense',   7120, '2026-07-03', 'Groceries',      'Groceries'),
  ('expense',   4200, '2026-07-05', 'Entertainment',  'Concert ticket'),
  ('expense',   4475, '2026-07-06', 'Transportation', 'Gas fill-up'),
  ('expense',   4000, '2026-07-07', 'Health',         'Doctor visit copay'),
  ('expense',   1599, '2026-07-11', 'Subscriptions',  'Netflix'),
  ('expense',   2215, '2026-07-12', 'Dining',         'Brunch with friends'),
  ('expense',   4933, '2026-07-13', 'Groceries',      'Groceries');

-- Budgets (monthly limits per category) ------------------------------------
INSERT INTO budgets (category, amount_cents) VALUES
  ('Groceries',      30000),
  ('Dining',         12000),
  ('Transportation', 10000),
  ('Entertainment',   3000),
  ('Utilities',      22000),
  ('Rent',          135000);

-- Recurring transaction rules ----------------------------------------------
INSERT INTO recurring (type, amount_cents, category, description, day_of_month) VALUES
  ('expense', 135000, 'Rent',          'Monthly rent', 1),
  ('expense',   1599, 'Subscriptions', 'Netflix',      19);

-- Bills --------------------------------------------------------------------
INSERT INTO bills (payee, amount_cents, frequency, category, due_day) VALUES
  ('Rent',             135000, 'monthly',   'Rent',          1),
  ('Electric Company',   7834, 'monthly',   'Utilities',     20),
  ('Internet',           6999, 'monthly',   'Utilities',     2),
  ('Netflix',            1599, 'monthly',   'Subscriptions', 19),
  ('Cell Phone',         5500, 'monthly',   'Phone',         2),
  ('Water',              4500, 'monthly',   'Utilities',     28),
  ('Car Insurance',     36000, 'quarterly', 'Insurance',     1),
  ('Domain Renewal',     1800, 'yearly',    'Subscriptions', 5);
