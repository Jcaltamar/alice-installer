CREATE TABLE legacy_records (id integer PRIMARY KEY, source text NOT NULL);
INSERT INTO legacy_records (id, source) VALUES (1, 'sanitized-fixture');
