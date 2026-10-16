DROP TABLE IF EXISTS case_contacts;
DROP TABLE IF EXISTS customer_reports;
ALTER TABLE customer_accounts DROP CONSTRAINT IF EXISTS customer_accounts_tail, DROP COLUMN IF EXISTS tail;
