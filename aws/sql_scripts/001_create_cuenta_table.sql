-- Drop the table if it already exists (only recommended for dev/testing)
DROP TABLE IF EXISTS cuenta;

-- Create the cuenta table
CREATE TABLE cuenta (
    id SERIAL PRIMARY KEY,
    date DATE NOT NULL,
    transaction TEXT NOT NULL,
    email TEXT NOT NULL
);
