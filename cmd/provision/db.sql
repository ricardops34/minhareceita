DO
$$
BEGIN
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'etl') THEN
      CREATE ROLE etl WITH LOGIN;
   END IF;
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'web') THEN
      CREATE ROLE web WITH LOGIN;
   END IF;
END
$$;

ALTER ROLE etl WITH PASSWORD '{{ .ETL }}';
ALTER ROLE web WITH PASSWORD '{{ .Web }}';

SELECT 'CREATE DATABASE minhareceita{{ .Suffix }} OWNER etl'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'minhareceita{{ .Suffix }}')
\gexec

GRANT ALL PRIVILEGES ON DATABASE minhareceita{{ .Suffix }} TO etl;
GRANT CONNECT ON DATABASE minhareceita{{ .Suffix }} TO web;

\c minhareceita{{ .Suffix }}
GRANT ALL ON SCHEMA public TO etl;
GRANT USAGE ON SCHEMA public TO web;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO web;
ALTER DEFAULT PRIVILEGES FOR ROLE etl IN SCHEMA public GRANT SELECT ON TABLES TO web;
ALTER DEFAULT PRIVILEGES FOR ROLE etl IN SCHEMA public GRANT ALL ON TABLES TO etl;
