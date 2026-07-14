-- Persist the order used by the board's manual sort mode. Existing tickets
-- retain their historical number order.
ALTER TABLE tickets ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
UPDATE tickets SET position = number WHERE position = 0;
CREATE INDEX idx_tickets_project_status_position
    ON tickets(project_id, status, position, number);
