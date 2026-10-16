DROP TRIGGER kill_switch_dual_control ON kill_switch_events;
DROP FUNCTION kill_switch_check();
DROP TRIGGER kill_switch_no_truncate ON kill_switch_events;
DROP TRIGGER kill_switch_no_change ON kill_switch_events;
DROP FUNCTION kill_switch_reject_change();
DROP TABLE kill_switch_events;
