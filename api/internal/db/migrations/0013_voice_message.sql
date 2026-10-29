-- B3: a player message can be the transcript of a push-to-talk clip.
ALTER TABLE match_message DROP CONSTRAINT match_message_kind_check;
ALTER TABLE match_message ADD CONSTRAINT match_message_kind_check CHECK (kind IN ('text', 'voice', 'answer', 'followup'));
