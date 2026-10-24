-- Telefonbuch: Grundschema
CREATE TABLE IF NOT EXISTS employees (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  username       VARCHAR(128)  NULL,
  last_name      VARCHAR(100)  NOT NULL DEFAULT '',
  first_name     VARCHAR(100)  NOT NULL DEFAULT '',
  phone          VARCHAR(64)   NOT NULL DEFAULT '',
  mobile         VARCHAR(64)   NOT NULL DEFAULT '',
  room           VARCHAR(64)   NOT NULL DEFAULT '',
  amt            VARCHAR(150)  NOT NULL DEFAULT '',
  email          VARCHAR(254)  NOT NULL DEFAULT '',
  source         VARCHAR(10)   NOT NULL DEFAULT 'manual',
  ad_guid        CHAR(36)      NULL,
  ad_dn          VARCHAR(512)  NULL,
  active         TINYINT(1)    NOT NULL DEFAULT 1,
  hidden         TINYINT(1)    NOT NULL DEFAULT 0,
  avatar         TEXT          NULL,
  created_at     DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at     DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_synced_at DATETIME(3)   NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_employees_username (username),
  UNIQUE KEY uq_employees_guid (ad_guid),
  KEY idx_employees_name (last_name, first_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tags (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name       VARCHAR(60)  NOT NULL,
  category   VARCHAR(20)  NOT NULL DEFAULT 'taetigkeit',
  created_by VARCHAR(128) NOT NULL DEFAULT '',
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_tags_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS employee_tags (
  employee_id BIGINT UNSIGNED NOT NULL,
  tag_id      BIGINT UNSIGNED NOT NULL,
  created_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (employee_id, tag_id),
  KEY idx_employee_tags_tag (tag_id),
  CONSTRAINT fk_et_employee FOREIGN KEY (employee_id) REFERENCES employees (id) ON DELETE CASCADE,
  CONSTRAINT fk_et_tag FOREIGN KEY (tag_id) REFERENCES tags (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS private_contacts (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  owner      VARCHAR(128) NOT NULL,
  last_name  VARCHAR(100) NOT NULL DEFAULT '',
  first_name VARCHAR(100) NOT NULL DEFAULT '',
  phone      VARCHAR(64)  NOT NULL DEFAULT '',
  mobile     VARCHAR(64)  NOT NULL DEFAULT '',
  room       VARCHAR(64)  NOT NULL DEFAULT '',
  amt        VARCHAR(150) NOT NULL DEFAULT '',
  email      VARCHAR(254) NOT NULL DEFAULT '',
  note       VARCHAR(500) NOT NULL DEFAULT '',
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_private_owner (owner)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS local_users (
  username     VARCHAR(128) NOT NULL,
  display_name VARCHAR(200) NOT NULL DEFAULT '',
  pw_hash      VARCHAR(255) NOT NULL,
  is_admin     TINYINT(1)   NOT NULL DEFAULT 0,
  disabled     TINYINT(1)   NOT NULL DEFAULT 0,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_login   DATETIME(3)  NULL,
  PRIMARY KEY (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sessions (
  id_hash      CHAR(64)     NOT NULL,
  username     VARCHAR(128) NOT NULL,
  display_name VARCHAR(200) NOT NULL DEFAULT '',
  is_admin     TINYINT(1)   NOT NULL DEFAULT 0,
  auth_source  VARCHAR(10)  NOT NULL DEFAULT '',
  csrf         CHAR(64)     NOT NULL,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_seen    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  expires_at   DATETIME(3)  NOT NULL,
  ip           VARCHAR(45)  NOT NULL DEFAULT '',
  user_agent   VARCHAR(255) NOT NULL DEFAULT '',
  PRIMARY KEY (id_hash),
  KEY idx_sessions_expires (expires_at),
  KEY idx_sessions_user (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS audit_log (
  id     BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  ts     DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  actor  VARCHAR(128) NOT NULL DEFAULT '',
  action VARCHAR(40)  NOT NULL,
  target VARCHAR(255) NOT NULL DEFAULT '',
  detail TEXT         NULL,
  ip     VARCHAR(45)  NOT NULL DEFAULT '',
  PRIMARY KEY (id),
  KEY idx_audit_ts (ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sync_runs (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  started_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  finished_at DATETIME(3)  NULL,
  trigger_by  VARCHAR(128) NOT NULL DEFAULT '',
  status      VARCHAR(10)  NOT NULL DEFAULT 'running',
  seen        INT NOT NULL DEFAULT 0,
  created     INT NOT NULL DEFAULT 0,
  updated     INT NOT NULL DEFAULT 0,
  deactivated INT NOT NULL DEFAULT 0,
  reactivated INT NOT NULL DEFAULT 0,
  message     TEXT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
