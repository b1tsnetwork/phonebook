-- Bevorzugter Kontaktweg je Organisationseinheit (Ticketsystem, Funktionspostfach, Hotline)
ALTER TABLE org_units ADD COLUMN contact TEXT NULL;
