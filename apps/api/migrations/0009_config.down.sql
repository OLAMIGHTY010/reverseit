DROP TRIGGER config_decisions_maker_checker ON config_decisions;
DROP FUNCTION config_decision_check();
DROP TRIGGER config_decisions_no_change ON config_decisions;
DROP TRIGGER config_versions_no_change ON config_versions;
DROP FUNCTION config_reject_change();
DROP TABLE config_decisions;
DROP TABLE config_versions;
