-- Zuständigkeitsfinder: Aufgaben mit Namenstrennung (Regeln als JSON) und Sprechzeiten
CREATE TABLE IF NOT EXISTS tasks (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name        VARCHAR(150)  NOT NULL,
  keywords    VARCHAR(500)  NOT NULL DEFAULT '',
  description VARCHAR(1000) NOT NULL DEFAULT '',
  org_unit_id BIGINT UNSIGNED NULL,
  hours       TEXT NULL,
  rules       MEDIUMTEXT NULL,
  sort        INT NOT NULL DEFAULT 0,
  updated_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE org_units ADD COLUMN hours TEXT NULL;
