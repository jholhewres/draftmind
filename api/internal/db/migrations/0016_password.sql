-- D1b: sign-in with an e-mail and a password (ADR-0010). The password is
-- kept only as an argon2id hash in PHC form, its parameters inside. An
-- account from a magic link or Steam has none.
ALTER TABLE app_user
    ADD COLUMN password_hash text CHECK (password_hash IS NULL OR email IS NOT NULL);
