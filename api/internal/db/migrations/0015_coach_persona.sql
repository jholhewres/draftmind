-- B5: the signed-in player's coach persona. It shapes only how the writer
-- words the coach's answers, never what is decided.
CREATE TABLE coach_persona (
    user_id    bigint PRIMARY KEY REFERENCES app_user (id) ON DELETE CASCADE,
    tone       text NOT NULL CHECK (tone IN ('direct', 'encouraging', 'analytic')),
    language   text NOT NULL CHECK (language IN ('auto', 'en', 'pt-BR')),
    verbosity  text NOT NULL CHECK (verbosity IN ('short', 'normal')),
    focus      text NOT NULL CHECK (focus IN ('laning', 'teamfights', 'items', 'macro')),
    updated_at timestamptz NOT NULL DEFAULT now()
);
