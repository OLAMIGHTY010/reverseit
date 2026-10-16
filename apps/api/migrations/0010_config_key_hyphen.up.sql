-- Calendar keys carry a date, such as calendar.holiday.2026-10-01, so a key may
-- contain a hyphen. The registry in application code still decides which keys
-- are valid; this constraint only keeps obviously malformed text out.
ALTER TABLE config_versions DROP CONSTRAINT config_versions_key_format;
ALTER TABLE config_versions ADD CONSTRAINT config_versions_key_format
    CHECK (key ~ '^[a-z][A-Za-z0-9_.-]{1,100}$');
