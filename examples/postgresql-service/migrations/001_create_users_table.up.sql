-- Create users table for the example PostgreSQL service
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create an index on email for faster lookups
CREATE INDEX idx_users_email ON users(email);

-- Create an index on created_at for time-based queries
CREATE INDEX idx_users_created_at ON users(created_at);