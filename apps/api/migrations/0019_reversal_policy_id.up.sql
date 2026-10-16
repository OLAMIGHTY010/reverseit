-- A reversal decision names the policy that governed it by a stable id, and records
-- the version of that policy separately (policy_version, which already exists and is
-- never changed after the decision). The id is reversal.<exception code>.<rail>,
-- which is also what a policy-scope kill switch freeze targets.
ALTER TABLE reversals ADD COLUMN policy_id text NOT NULL DEFAULT '';

UPDATE reversals r SET policy_id = 'reversal.' || c.classification || '.' || t.rail
  FROM cases c, transactions t
 WHERE c.tenant_id = r.tenant_id AND c.id = r.case_id AND t.tenant_id = r.tenant_id AND t.id = r.txn_id;
