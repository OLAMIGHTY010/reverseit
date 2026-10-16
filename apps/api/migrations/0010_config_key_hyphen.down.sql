ALTER TABLE config_versions DROP CONSTRAINT config_versions_key_format;
ALTER TABLE config_versions ADD CONSTRAINT config_versions_key_format
    CHECK (key ~ '^[a-z][A-Za-z0-9_.]{1,100}$');
