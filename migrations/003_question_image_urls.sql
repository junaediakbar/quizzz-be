-- Optional image URLs (Cloudinary or any HTTPS URL) per question, JSON array of strings
ALTER TABLE questions
    ADD COLUMN IF NOT EXISTS image_urls JSONB;
