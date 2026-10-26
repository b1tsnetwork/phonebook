-- Persönliche Einstellungen (Favoriten, Extras) je Benutzer
CREATE TABLE IF NOT EXISTS user_prefs (
  username   VARCHAR(128) NOT NULL,
  prefs      TEXT         NOT NULL,
  updated_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
