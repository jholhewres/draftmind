-- D1b: sign-in with an e-mail and a password (ADR-0010). The password is
-- kept only as an argon2id hash in PHC form, its parameters inside. An
-- account from a magic link or Steam has none.
ALTER TABLE app_user
    ADD COLUMN password_hash text CHECK (password_hash IS NULL OR email IS NOT NULL),
    -- Set when a magic link proved the address. A password set before
    -- that is dropped by the first link, so registering someone else's
    -- address cannot hold their account.
    ADD COLUMN email_verified_at timestamptz;

-- Every e-mail account so far came from a magic link.
UPDATE app_user SET email_verified_at = created_at WHERE email IS NOT NULL;
