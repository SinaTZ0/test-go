CREATE TABLE IF NOT EXISTS todos (
	id BIGSERIAL PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT,
	completed BOOLEAN NOT NULL DEFAULT FALSE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);

DO $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM information_schema.columns
		WHERE table_name = 'todos'
			AND column_name = 'description'
			AND is_nullable = 'NO'
	) THEN
		ALTER TABLE todos ALTER COLUMN description DROP NOT NULL;
	END IF;

	IF EXISTS (
		SELECT 1
		FROM information_schema.columns
		WHERE table_name = 'todos'
			AND column_name = 'description'
			AND column_default IS NOT NULL
	) THEN
		ALTER TABLE todos ALTER COLUMN description DROP DEFAULT;
	END IF;
END $$;