-- An approval is given under the policy in force at the time. authority_basis lists
-- the versions of the authorisation settings it relied on (approval limits, validity,
-- automatic eligibility, the execution flag). If any of them has changed by the time
-- the reversal is submitted and the reversal still needs a person's approval, the
-- earlier approval no longer counts. policy_version is the full set of versions
-- consulted at approval, kept for the record.
ALTER TABLE approvals
    ADD COLUMN policy_version  text NOT NULL DEFAULT '',
    ADD COLUMN authority_basis text NOT NULL DEFAULT '';
