-- Create todos table
CREATE TABLE IF NOT EXISTS todos (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    completed BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Add index for faster queries
CREATE INDEX IF NOT EXISTS idx_todos_created_at ON todos(created_at DESC);