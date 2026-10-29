-- The signed-in player's UI language, so it follows them to a new device.
-- Null until they pick one; the browser language decides until then.
ALTER TABLE app_user ADD COLUMN ui_language text CHECK (ui_language IN ('en', 'pt-BR'));
