-- Initial database schema
CREATE TABLE IF NOT EXISTS tasks (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    completed BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create an index on created_at for better query performance
CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at);

-- Insert sample data
INSERT INTO tasks (title, description, completed) VALUES
('Setup Docker Environment', 'Configure Docker Compose for development', true),
('Implement API Endpoints', 'Create REST API for task management', false),
('Add Frontend Integration', 'Connect React frontend to API', false);