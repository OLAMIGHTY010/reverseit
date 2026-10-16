DROP POLICY IF EXISTS tenant_isolation ON reconciliation_breaks;
DROP POLICY IF EXISTS tenant_isolation ON settlement_records;
DROP POLICY IF EXISTS tenant_isolation ON settlement_files;
DROP TABLE IF EXISTS reconciliation_breaks;
DROP TABLE IF EXISTS settlement_records;
DROP TABLE IF EXISTS settlement_files;
