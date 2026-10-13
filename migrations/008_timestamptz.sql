-- 008: semua kolom waktu disimpan sebagai timestamptz (instant), bukan
-- "timestamp without time zone" yang dibaca lib/pq sebagai UTC.
--
-- Akibat sebelumnya: server (WIB) menulis 12:00 ke kolom tanpa zona, driver
-- membacanya sebagai 12:00Z, lalu frontend menghitung sisa waktu ujian dari
-- nilai itu → timer membengkak +7 jam (ujian 60 menit tampil 479 menit).
--
-- Nilai lama adalah wall-clock WIB, jadi direinterpretasi dengan AT TIME ZONE
-- 'Asia/Jakarta' agar instannya benar.

CREATE OR REPLACE FUNCTION quizzz_migrate_timestamps_to_tz() RETURNS void AS $$
DECLARE
    rec record;
BEGIN
    FOR rec IN
        SELECT table_name, column_name
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND data_type = 'timestamp without time zone'
    LOOP
        EXECUTE format(
            'ALTER TABLE %I ALTER COLUMN %I TYPE timestamptz USING %I AT TIME ZONE ''Asia/Jakarta''',
            rec.table_name, rec.column_name, rec.column_name
        );
    END LOOP;
END;
$$ LANGUAGE plpgsql;

SELECT quizzz_migrate_timestamps_to_tz();

DROP FUNCTION quizzz_migrate_timestamps_to_tz();
