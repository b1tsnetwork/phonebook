-- Kontaktwege als eigene Objekte (Ticketsystem/Postfach/Hotline), Personen werden gezielt zugewiesen
CREATE TABLE IF NOT EXISTS contact_channels (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name         VARCHAR(100) NOT NULL,
  ticket_url   VARCHAR(500) NOT NULL DEFAULT '',
  ticket_label VARCHAR(40)  NOT NULL DEFAULT '',
  email        VARCHAR(254) NOT NULL DEFAULT '',
  phone        VARCHAR(64)  NOT NULL DEFAULT '',
  note         VARCHAR(200) NOT NULL DEFAULT '',
  hide_direct  TINYINT(1)   NOT NULL DEFAULT 0,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE employees ADD COLUMN channel_id BIGINT UNSIGNED NULL;
