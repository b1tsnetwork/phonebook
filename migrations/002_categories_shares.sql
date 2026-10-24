-- Schlagwort-Kategorien (frei verwaltbar) und Freigaben privater Kontakte
CREATE TABLE IF NOT EXISTS tag_categories (
  cat_key    VARCHAR(40)  NOT NULL,
  name       VARCHAR(60)  NOT NULL,
  color      CHAR(7)      NOT NULL DEFAULT '#888888',
  sort       INT          NOT NULL DEFAULT 0,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (cat_key),
  UNIQUE KEY uq_tag_categories_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO tag_categories (cat_key, name, color, sort) VALUES
  ('taetigkeit', 'Tätigkeit', '#3b7dd8', 10),
  ('besonderheit', 'Besonderheit', '#d48a14', 20),
  ('sonstiges', 'Sonstiges', '#7c8591', 30);

CREATE TABLE IF NOT EXISTS private_shares (
  contact_id BIGINT UNSIGNED NOT NULL,
  kind       VARCHAR(10)  NOT NULL,
  value      VARCHAR(150) NOT NULL,
  PRIMARY KEY (contact_id, kind, value),
  KEY idx_private_shares_value (kind, value),
  CONSTRAINT fk_ps_contact FOREIGN KEY (contact_id) REFERENCES private_contacts (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
