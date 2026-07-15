# Requirements Document

## Introduction

The Budget Tracker is a web application that helps a single user record and review personal finances. Users can enter income and expense transactions, view aggregated summaries on a dashboard, browse chronological logs of entries, and organize views by time period (day, week, month). The application is built in Go with sqlc-generated data access against a SQLite database, and uses HTMX for a server-rendered, partially-updating frontend.

This document defines the functional and quality requirements for the feature. Technical implementation choices (Go, sqlc, SQLite, HTMX) are treated as fixed constraints from the user and are reflected where they affect observable system behavior.

## Glossary

- **Budget_Tracker**: The complete web application, including HTTP server, request handlers, and rendered HTML responses.
- **Transaction**: A single financial record of type expense or income, with an amount, date, category, and optional description.
- **Expense**: A Transaction that reduces available funds.
- **Income**: A Transaction that increases available funds.
- **Amount**: A monetary value stored as a non-negative number with two decimal places, associated with a Transaction.
- **Category**: A user-assigned label that groups Transactions (for example, "Groceries", "Salary").
- **Transaction_Date**: The calendar date on which a Transaction occurred, as entered by the user.
- **Dashboard**: A rendered view that displays aggregated financial summaries for a selected time period.
- **Transaction_Log**: A rendered view that lists individual Transactions in chronological order.
- **Time_Period**: A grouping interval for viewing Transactions, one of day, week, or month.
- **Net_Balance**: The value computed as total Income minus total Expense over a selected Time_Period.
- **Data_Store**: The SQLite database and its sqlc-generated access layer used to persist and retrieve Transactions.
- **User**: The single person operating the Budget_Tracker.

## Requirements

### Requirement 1: Record an Expense

**User Story:** As a user, I want to enter an expense, so that I can track money I have spent.

#### Acceptance Criteria

1. WHEN a User submits an expense entry with a valid Amount, Transaction_Date, and Category, THE Budget_Tracker SHALL persist the Transaction as an Expense in the Data_Store, where a valid Amount is a value from 0.01 to 999,999,999.99 with at most two decimal places, a valid Transaction_Date is a calendar date in ISO 8601 format (YYYY-MM-DD), and a valid Category is a non-empty label of 1 to 100 characters.
2. IF a User submits an expense entry with an Amount that is missing, non-numeric, less than or equal to 0, greater than 999,999,999.99, or has more than two decimal places, THEN THE Budget_Tracker SHALL reject the entry, preserve all previously stored Transactions unchanged, and return a validation message identifying the Amount field.
3. IF a User submits an expense entry with a missing Transaction_Date, THEN THE Budget_Tracker SHALL reject the entry, preserve all previously stored Transactions unchanged, and return a validation message identifying the Transaction_Date field.
4. IF a User submits an expense entry with a missing Category, THEN THE Budget_Tracker SHALL reject the entry, preserve all previously stored Transactions unchanged, and return a validation message identifying the Category field.
5. IF a User submits an expense entry with a Transaction_Date that is not a valid calendar date in ISO 8601 format (YYYY-MM-DD), THEN THE Budget_Tracker SHALL reject the entry, preserve all previously stored Transactions unchanged, and return a validation message identifying the Transaction_Date field.
6. WHEN an Expense is successfully persisted, THE Budget_Tracker SHALL return an updated Transaction_Log fragment that includes the new Expense.

### Requirement 2: Record Income

**User Story:** As a user, I want to enter income, so that I can track money I have received.

#### Acceptance Criteria

1. WHEN a User submits an income entry with a valid Amount, Transaction_Date, and Category, THE Budget_Tracker SHALL persist the Transaction as Income in the Data_Store, where a valid Amount is a value from 0.01 to 999,999,999.99 with at most two decimal places, a valid Transaction_Date is a calendar date in ISO 8601 format (YYYY-MM-DD), and a valid Category is a non-empty label of 1 to 100 characters.
2. IF a User submits an income entry with an Amount that is missing, non-numeric, less than or equal to 0, greater than 999,999,999.99, or has more than two decimal places, THEN THE Budget_Tracker SHALL reject the entry, preserve all previously stored Transactions unchanged, and return a validation message identifying the Amount field.
3. IF a User submits an income entry with a missing Transaction_Date, THEN THE Budget_Tracker SHALL reject the entry, preserve all previously stored Transactions unchanged, and return a validation message identifying the Transaction_Date field.
4. IF a User submits an income entry with a Transaction_Date that is not a valid calendar date in ISO 8601 format (YYYY-MM-DD), THEN THE Budget_Tracker SHALL reject the entry, preserve all previously stored Transactions unchanged, and return a validation message identifying the Transaction_Date field.
5. IF a User submits an income entry with a missing Category, THEN THE Budget_Tracker SHALL reject the entry, preserve all previously stored Transactions unchanged, and return a validation message identifying the Category field.
6. WHEN Income is successfully persisted, THE Budget_Tracker SHALL return an updated Transaction_Log fragment that includes the new Income.

### Requirement 3: Edit a Transaction

**User Story:** As a user, I want to edit a transaction I already entered, so that I can correct mistakes.

#### Acceptance Criteria

1. WHEN a User submits an edit to an existing Transaction with a valid Amount greater than 0, a Transaction_Date that is a valid calendar date in ISO 8601 format (YYYY-MM-DD), and a non-empty Category, THE Budget_Tracker SHALL update the stored Transaction in the Data_Store.
2. IF a User submits an edit referencing a Transaction identifier that does not exist in the Data_Store, THEN THE Budget_Tracker SHALL leave all stored Transactions unchanged and return an error message indicating the Transaction was not found.
3. IF a User submits an edit with an Amount less than or equal to 0, THEN THE Budget_Tracker SHALL reject the edit, leave the stored Transaction unchanged in the Data_Store, and return a validation message identifying the Amount field.
4. IF a User submits an edit with a missing Transaction_Date, a Transaction_Date that is not a valid calendar date in ISO 8601 format (YYYY-MM-DD), or a missing Category, THEN THE Budget_Tracker SHALL reject the entire edit, leave the stored Transaction unchanged in the Data_Store, and return a validation message identifying each invalid field.
5. WHEN a Transaction is successfully updated, THE Budget_Tracker SHALL return an updated Transaction_Log fragment that reflects the updated Transaction values.

### Requirement 4: Delete a Transaction

**User Story:** As a user, I want to delete a transaction, so that I can remove entries I no longer want.

#### Acceptance Criteria

1. WHEN a User confirms deletion of an existing Transaction, THE Budget_Tracker SHALL remove the Transaction from the Data_Store so that the Transaction is no longer retrievable in subsequent requests.
2. IF a User requests deletion of a Transaction identifier that does not exist in the Data_Store, THEN THE Budget_Tracker SHALL return an error message indicating the Transaction was not found and preserve all stored Transactions unchanged.
3. WHEN a Transaction is successfully deleted, THE Budget_Tracker SHALL return an updated Transaction_Log fragment that excludes the deleted Transaction and retains all other Transactions.
4. IF a Data_Store operation fails during deletion of a Transaction, THEN THE Budget_Tracker SHALL return an error response indicating the deletion did not complete and preserve the target Transaction unchanged.

### Requirement 5: View Dashboard Summary

**User Story:** As a user, I want to see a summary of my finances, so that I can understand my current financial position.

#### Acceptance Criteria

1. WHEN a User opens the Dashboard, THE Budget_Tracker SHALL display the total Income, total Expense, and Net_Balance for the selected Time_Period, each formatted to exactly two decimal places.
2. THE Budget_Tracker SHALL compute Net_Balance as total Income minus total Expense over all Transactions whose Transaction_Date falls within the selected Time_Period, where a negative Net_Balance is valid when total Expense exceeds total Income.
3. WHERE no Transactions exist for the selected Time_Period, THE Budget_Tracker SHALL display total Income of 0.00, total Expense of 0.00, and Net_Balance of 0.00.
4. WHEN a User opens the Dashboard, THE Budget_Tracker SHALL display total Expense grouped by Category for the selected Time_Period, with exactly one entry per Category that has at least one Expense and no entry for Categories with no Expense.
5. WHERE no Time_Period is selected when the Dashboard is opened, THE Budget_Tracker SHALL apply a default Time_Period of month.
6. IF the Budget_Tracker fails to compute the financial totals when opening the Dashboard, THEN THE Budget_Tracker SHALL display an indicator that the totals are unavailable and preserve all stored Transactions unchanged.

### Requirement 6: View Transaction Log

**User Story:** As a user, I want to see a log of my transactions, so that I can review individual entries.

#### Acceptance Criteria

1. WHEN a User opens the Transaction_Log, THE Budget_Tracker SHALL display each Transaction with its Amount formatted to two decimal places, its Category, its Transaction_Date in ISO 8601 format (YYYY-MM-DD), its type as either Expense or Income, and its description.
2. WHEN a User opens the Transaction_Log and a displayed Transaction has no description, THE Budget_Tracker SHALL display that Transaction with an empty description field and all other Transaction values present.
3. THE Budget_Tracker SHALL order Transactions in the Transaction_Log by Transaction_Date in descending order, from latest to earliest, by default.
4. WHERE no Transactions exist, THE Budget_Tracker SHALL display the Transaction_Log interface containing a message indicating that no Transactions exist.
5. WHERE a User filters the Transaction_Log by a transaction type of Expense or Income, THE Budget_Tracker SHALL display only Transactions matching the selected type while preserving the default descending Transaction_Date order.
6. WHERE no transaction type filter is selected, THE Budget_Tracker SHALL display Transactions of both Expense and Income types.
7. IF the Budget_Tracker fails to retrieve Transactions from the Data_Store when a User opens the Transaction_Log, THEN THE Budget_Tracker SHALL return an error message indicating the Transaction_Log could not be loaded and SHALL not display partial Transaction results.

### Requirement 7: Group Transactions by Time Period

**User Story:** As a user, I want to view my transactions grouped by day, week, or month, so that I can analyze my spending over time.

#### Acceptance Criteria

1. WHEN a User selects a Time_Period of day, THE Budget_Tracker SHALL group and display Transactions by individual Transaction_Date, with groups ordered from most recent date to least recent date.
2. WHEN a User selects a Time_Period of week, THE Budget_Tracker SHALL group and display Transactions by calendar week starting on Monday, with groups ordered from most recent week to least recent week.
3. WHEN a User selects a Time_Period of month, THE Budget_Tracker SHALL group and display Transactions by calendar month, with groups ordered from most recent month to least recent month.
4. WHEN a User selects a Time_Period, THE Budget_Tracker SHALL display for each group the total Income, total Expense, and Net_Balance, where each group's Net_Balance is that group's total Income minus that group's total Expense.
5. IF the Budget_Tracker groups Transactions for a selected Time_Period but fails to compute the financial totals, THEN THE Budget_Tracker SHALL display the grouped Transactions and an indicator that the totals are unavailable.
6. WHERE no Transactions exist for the selected Time_Period, THE Budget_Tracker SHALL display a message indicating that no Transactions exist for that Time_Period.
7. WHERE no Time_Period is selected, THE Budget_Tracker SHALL default to a Time_Period of month.
8. IF a User selects a Time_Period value that is not day, week, or month, THEN THE Budget_Tracker SHALL default to a Time_Period of month.

### Requirement 8: Sort Transaction Views

**User Story:** As a user, I want to sort my transaction views, so that I can find entries in the order most useful to me.

#### Acceptance Criteria

1. WHEN a User selects sort by Transaction_Date ascending, THE Budget_Tracker SHALL display Transactions ordered from earliest to latest Transaction_Date.
2. WHEN a User selects sort by Transaction_Date descending, THE Budget_Tracker SHALL display Transactions ordered from latest to earliest Transaction_Date.
3. WHEN a User selects sort by Amount ascending, THE Budget_Tracker SHALL display Transactions ordered from lowest to highest Amount.
4. WHEN a User selects sort by Amount descending, THE Budget_Tracker SHALL display Transactions ordered from highest to lowest Amount.
5. WHEN a User applies a sort selection, THE Budget_Tracker SHALL preserve any active Time_Period and type filters in the displayed result.
6. WHEN two or more Transactions have equal values for the selected sort field, THE Budget_Tracker SHALL order those Transactions by their Transaction identifier in ascending order.
7. IF a User applies a sort selection that is not a recognized sort field or direction, THEN THE Budget_Tracker SHALL display Transactions in the default Transaction_Date descending order.

### Requirement 9: Persist Data Across Sessions

**User Story:** As a user, I want my data to be saved, so that my transactions remain available when I return to the application.

#### Acceptance Criteria

1. WHEN a User creates, edits, or deletes a Transaction, THE Budget_Tracker SHALL durably commit the change to the SQLite Data_Store before returning a success response, so that the change is retrievable after the server restarts.
2. WHEN the Budget_Tracker starts and the Data_Store schema is absent or incomplete, THE Budget_Tracker SHALL create the required schema before accepting Transaction requests.
3. WHEN schema verification or creation succeeds during startup, THE Budget_Tracker SHALL begin accepting Transaction requests.
4. IF schema creation fails during startup, THEN THE Budget_Tracker SHALL refuse all Transaction requests and return an error response indicating the Data_Store is unavailable until the failure is resolved by manual intervention.
5. IF a Data_Store operation fails, THEN THE Budget_Tracker SHALL return an error response indicating the operation could not be completed and preserve the previously stored data unchanged with no partial write persisted.

### Requirement 10: Serve the Web Interface

**User Story:** As a user, I want to interact with the application through a web browser, so that I can manage my budget without a separate client.

#### Acceptance Criteria

1. WHEN a User requests the application root path, THE Budget_Tracker SHALL return a complete HTML page that renders both the Dashboard and the Transaction_Log.
2. WHEN a User performs a create, edit, or delete action through the web interface, THE Budget_Tracker SHALL return an HTML fragment that re-renders only the Transaction_Log and the Dashboard summary affected by the action, without returning a full page.
3. IF a requested route does not exist, THEN THE Budget_Tracker SHALL return an HTTP 404 response with an error page indicating the requested resource was not found.
4. IF the Budget_Tracker cannot render the requested root page because a Data_Store operation fails, THEN THE Budget_Tracker SHALL return an error page indicating the page could not be loaded and SHALL leave the stored data unchanged.
