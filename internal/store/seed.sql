-- Seed data for the Budget Tracker: a coherent demo dataset for one person
-- covering May–July 2026, plus budgets, recurring rules, and bills.
--
-- Rows are inserted with a placeholder user_id of 0; Repo.Seed reassigns them
-- to the target user after loading this file. Users and sessions are left
-- untouched. This script clears existing financial data first, so applying it
-- always yields the same known state. All amounts are integer cents.

DELETE FROM recurring_postings;
DELETE FROM transactions;
DELETE FROM budgets;
DELETE FROM recurring;
DELETE FROM bills;

-- Transactions -------------------------------------------------------------
INSERT INTO transactions (user_id, type, amount_cents, date, category, description) VALUES
  -- May 2026
  (0, 'income',  230000, '2026-05-01', 'Salary',         'Biweekly paycheck'),
  (0, 'income',  230000, '2026-05-15', 'Salary',         'Biweekly paycheck'),
  (0, 'expense', 135000, '2026-05-01', 'Rent',           'Monthly rent'),
  (0, 'expense',   6999, '2026-05-02', 'Utilities',      'Internet service'),
  (0, 'expense',   8240, '2026-05-05', 'Groceries',      'Grocery run'),
  (0, 'expense',   2460, '2026-05-09', 'Dining',         'Dinner out'),
  (0, 'expense',   4500, '2026-05-12', 'Transportation', 'Gas fill-up'),
  (0, 'expense',   1599, '2026-05-15', 'Subscriptions',  'Netflix'),
  (0, 'expense',   6125, '2026-05-18', 'Groceries',      'Groceries'),
  (0, 'expense',   7834, '2026-05-20', 'Utilities',      'Electricity bill'),
  (0, 'expense',   3000, '2026-05-24', 'Entertainment',  'Concert ticket'),
  (0, 'expense',   2500, '2026-05-28', 'Health',         'Pharmacy'),
  -- June 2026
  (0, 'income',  230000, '2026-06-12', 'Salary',         'Biweekly paycheck'),
  (0, 'income',  230000, '2026-06-26', 'Salary',         'Biweekly paycheck'),
  (0, 'income',   45000, '2026-06-28', 'Freelance',      'Freelance project'),
  (0, 'expense', 135000, '2026-06-01', 'Rent',           'Monthly rent'),
  (0, 'expense',   6999, '2026-06-02', 'Utilities',      'Internet service'),
  (0, 'expense',   7410, '2026-06-05', 'Groceries',      'Groceries'),
  (0, 'expense',   1875, '2026-06-10', 'Dining',         'Lunch with coworker'),
  (0, 'expense',   4475, '2026-06-15', 'Transportation', 'Gas fill-up'),
  (0, 'expense',   1599, '2026-06-19', 'Subscriptions',  'Netflix'),
  (0, 'expense',   7834, '2026-06-20', 'Utilities',      'Electricity bill'),
  (0, 'expense',   8999, '2026-06-22', 'Shopping',       'New shoes'),
  (0, 'expense',   3280, '2026-06-25', 'Dining',         'Dinner out'),
  (0, 'expense',   4500, '2026-06-29', 'Utilities',      'Water bill'),
  -- July 2026
  (0, 'income',  230000, '2026-07-10', 'Salary',         'Biweekly paycheck'),
  (0, 'income',    1235, '2026-07-10', 'Interest',       'Savings account interest'),
  (0, 'expense', 135000, '2026-07-01', 'Rent',           'Monthly rent'),
  (0, 'expense',   6999, '2026-07-02', 'Utilities',      'Internet service'),
  (0, 'expense',   5500, '2026-07-02', 'Phone',          'Cell phone bill'),
  (0, 'expense',   7120, '2026-07-03', 'Groceries',      'Groceries'),
  (0, 'expense',   4200, '2026-07-05', 'Entertainment',  'Concert ticket'),
  (0, 'expense',   4475, '2026-07-06', 'Transportation', 'Gas fill-up'),
  (0, 'expense',   4000, '2026-07-07', 'Health',         'Doctor visit copay'),
  (0, 'expense',   1599, '2026-07-11', 'Subscriptions',  'Netflix'),
  (0, 'expense',   2215, '2026-07-12', 'Dining',         'Brunch with friends'),
  (0, 'expense',   4933, '2026-07-13', 'Groceries',      'Groceries');

-- Budgets (monthly limits per category) ------------------------------------
INSERT INTO budgets (user_id, category, amount_cents) VALUES
  (0, 'Groceries',      30000),
  (0, 'Dining',         12000),
  (0, 'Transportation', 10000),
  (0, 'Entertainment',   3000),
  (0, 'Utilities',      22000),
  (0, 'Rent',          135000);

-- Recurring transaction rules ----------------------------------------------
INSERT INTO recurring (user_id, type, amount_cents, category, description, day_of_month) VALUES
  (0, 'expense', 135000, 'Rent',          'Monthly rent', 1),
  (0, 'expense',   1599, 'Subscriptions', 'Netflix',      19);

-- Bills --------------------------------------------------------------------
INSERT INTO bills (user_id, payee, amount_cents, frequency, category, due_day) VALUES
  (0, 'Rent',             135000, 'monthly',   'Rent',          1),
  (0, 'Electric Company',   7834, 'monthly',   'Utilities',     20),
  (0, 'Internet',           6999, 'monthly',   'Utilities',     2),
  (0, 'Netflix',            1599, 'monthly',   'Subscriptions', 19),
  (0, 'Cell Phone',         5500, 'monthly',   'Phone',         2),
  (0, 'Water',              4500, 'monthly',   'Utilities',     28),
  (0, 'Car Insurance',     36000, 'quarterly', 'Insurance',     1),
  (0, 'Domain Renewal',     1800, 'yearly',    'Subscriptions', 5);
