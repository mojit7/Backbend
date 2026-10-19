-- Simple infrastructure key/value table, managed by the platform module.
CREATE TABLE platform_settings (
    key        text PRIMARY KEY,
    value      text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
