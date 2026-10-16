-- The bank's own target for a case, kept apart from the regulatory deadline in sla_due_at.
ALTER TABLE cases ADD COLUMN internal_target_at timestamptz;
