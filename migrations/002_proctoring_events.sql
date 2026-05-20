-- Proctoring / integrity events per PRD (tab switch, fullscreen exit, etc.)
CREATE TABLE IF NOT EXISTS exam_proctoring_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID NOT NULL REFERENCES exam_sessions(id) ON DELETE CASCADE,
    event_type VARCHAR(64) NOT NULL,
    detail JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_proctoring_session ON exam_proctoring_events(session_id);
