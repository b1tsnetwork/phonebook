-- Organisation (Ebenen/Einheiten), Vertretungen, Abwesenheiten, Fehlermeldungen
ALTER TABLE employees ADD COLUMN org_unit_id BIGINT UNSIGNED NULL;
ALTER TABLE employees ADD COLUMN deputies TEXT NULL;
ALTER TABLE employees ADD COLUMN absences TEXT NULL;
CREATE INDEX idx_employees_org ON employees (org_unit_id);

CREATE TABLE IF NOT EXISTS org_units (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  parent_id  BIGINT UNSIGNED NULL,
  name       VARCHAR(150) NOT NULL,
  short_name VARCHAR(30)  NOT NULL DEFAULT '',
  sort       INT          NOT NULL DEFAULT 0,
  leaders    TEXT         NULL,
  deputies   TEXT         NULL,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_org_units_parent (parent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS settings (
  k VARCHAR(64) NOT NULL,
  v TEXT        NOT NULL,
  PRIMARY KEY (k)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS reports (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  employee_id   BIGINT UNSIGNED NULL,
  target        VARCHAR(255) NOT NULL DEFAULT '',
  field         VARCHAR(20)  NOT NULL,
  suggestion    VARCHAR(255) NOT NULL DEFAULT '',
  message       VARCHAR(500) NOT NULL DEFAULT '',
  reporter      VARCHAR(128) NOT NULL,
  reporter_name VARCHAR(200) NOT NULL DEFAULT '',
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  status        VARCHAR(10)  NOT NULL DEFAULT 'open',
  resolved_by   VARCHAR(128) NOT NULL DEFAULT '',
  resolved_at   DATETIME(3)  NULL,
  note          VARCHAR(500) NOT NULL DEFAULT '',
  PRIMARY KEY (id),
  KEY idx_reports_status (status, created_at),
  KEY idx_reports_employee (employee_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
