-- Add operator role support and page-level admin access grants.

ALTER TABLE users
  DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE users
  ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'operator', 'user'));

CREATE TABLE IF NOT EXISTS operator_page_permissions (
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  page_key VARCHAR(100) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, page_key)
);

CREATE INDEX IF NOT EXISTS idx_operator_page_permissions_page_key
  ON operator_page_permissions(page_key);
