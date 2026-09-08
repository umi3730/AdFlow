-- Opaque profile IDs must compare exactly as Redis keys and Go map keys do.
-- NO PAD also avoids the trailing-space aliases of utf8mb4_unicode_ci.
ALTER TABLE user_profiles MODIFY user_id VARCHAR(128)
    CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL;
-- Match the API's surrounding-whitespace normalization. A key collision fails
-- this statement instead of silently merging profiles.
UPDATE user_profiles SET user_id = REGEXP_REPLACE(user_id, '^[[:space:]]+|[[:space:]]+$', '');
