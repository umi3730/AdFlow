-- Fails rather than merging rows if case-distinct IDs have since been created.
ALTER TABLE user_profiles MODIFY user_id VARCHAR(128)
    CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL;
