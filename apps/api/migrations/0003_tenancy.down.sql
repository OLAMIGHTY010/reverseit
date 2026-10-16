DROP FUNCTION institution_id_by_code(text);
DROP TABLE user_roles;
DROP TABLE users;
DROP TABLE institutions;
-- The sr_app role is left in place: roles are cluster-wide and other
-- databases or later migrations may use it.
