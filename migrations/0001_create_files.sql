CREATE TABLE IF NOT EXISTS files (
    id           CHAR(36)     NOT NULL PRIMARY KEY,
    filename     VARCHAR(512) NOT NULL,
    content_type VARCHAR(255) NOT NULL,
    size         BIGINT       NOT NULL,
    storage_key  VARCHAR(512) NOT NULL,
    created_at   TIMESTAMP    NOT NULL,
    expires_at   TIMESTAMP    NULL,
    INDEX idx_files_expires_at (expires_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;
