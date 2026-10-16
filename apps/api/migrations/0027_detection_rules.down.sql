DROP TABLE IF EXISTS rule_evaluations;
DROP TRIGGER IF EXISTS detection_rules_fixed ON detection_rules;
DROP FUNCTION IF EXISTS detection_rules_definition_is_fixed();
DROP TABLE IF EXISTS detection_rules;
