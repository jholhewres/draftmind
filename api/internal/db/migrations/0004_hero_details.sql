-- Hero details from the game's data feed, per patch (ADR-0007).

-- Base stats; null until the details sync reaches the hero.
ALTER TABLE hero_patch
    ADD COLUMN complexity        smallint,
    ADD COLUMN str_base          integer,
    ADD COLUMN str_gain          double precision,
    ADD COLUMN agi_base          integer,
    ADD COLUMN agi_gain          double precision,
    ADD COLUMN int_base          integer,
    ADD COLUMN int_gain          double precision,
    ADD COLUMN armor             double precision,
    ADD COLUMN magic_resistance  integer,
    ADD COLUMN damage_min        integer,
    ADD COLUMN damage_max        integer,
    ADD COLUMN attack_rate       double precision,
    ADD COLUMN attack_range      integer,
    ADD COLUMN projectile_speed  integer,
    ADD COLUMN movement_speed    integer,
    ADD COLUMN turn_rate         double precision,
    ADD COLUMN max_health        integer,
    ADD COLUMN health_regen      double precision,
    ADD COLUMN max_mana          integer,
    ADD COLUMN mana_regen        double precision,
    ADD COLUMN sight_range_day   integer,
    ADD COLUMN sight_range_night integer;

-- A hero's text in one language (BCP 47 tag, e.g. en, pt-BR).
CREATE TABLE hero_text (
    patch_id integer NOT NULL,
    hero_id  integer NOT NULL,
    lang     text NOT NULL,
    name     text NOT NULL,
    hype     text NOT NULL,
    bio      text NOT NULL,
    PRIMARY KEY (patch_id, hero_id, lang),
    FOREIGN KEY (patch_id, hero_id) REFERENCES hero_patch (patch_id, hero_id) ON DELETE CASCADE
);

-- A hero's abilities, innate ability and talents on a patch, in feed order.
CREATE TABLE ability (
    patch_id   integer NOT NULL,
    hero_id    integer NOT NULL,
    id         integer NOT NULL,
    slot       smallint NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('ability', 'innate', 'talent')),
    name       text NOT NULL,
    cooldowns  double precision[] NOT NULL DEFAULT '{}',
    mana_costs double precision[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (patch_id, hero_id, id),
    FOREIGN KEY (patch_id, hero_id) REFERENCES hero_patch (patch_id, hero_id) ON DELETE CASCADE
);

CREATE TABLE ability_text (
    patch_id    integer NOT NULL,
    hero_id     integer NOT NULL,
    ability_id  integer NOT NULL,
    lang        text NOT NULL,
    name        text NOT NULL,
    description text NOT NULL,
    lore        text NOT NULL,
    PRIMARY KEY (patch_id, hero_id, ability_id, lang),
    FOREIGN KEY (patch_id, hero_id, ability_id) REFERENCES ability (patch_id, hero_id, id) ON DELETE CASCADE
);
