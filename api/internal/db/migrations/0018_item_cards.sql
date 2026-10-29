-- The item's image path from dotaconstants items.json, relative to the
-- game's image CDN, and the items a coach answer names, as the cards the
-- chat shows under it (id, key, name, cost, img from the item table).
ALTER TABLE item ADD COLUMN img text NOT NULL DEFAULT '';
ALTER TABLE match_message ADD COLUMN items jsonb;
