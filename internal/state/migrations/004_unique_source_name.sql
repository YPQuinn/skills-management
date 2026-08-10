-- Source names are operator-facing identity (one URL path segment, unique
-- per browser contract). Earlier builds allowed duplicate display names;
-- this migration renames later duplicates deterministically (keeping the
-- lowest id per name and suffixing the others with their id) and then
-- enforces uniqueness with a unique index. A residual collision with an
-- existing name that already equals a suffixed form fails the migration
-- loudly rather than guessing.
UPDATE sources
SET name = name || '-' || id
WHERE id IN (
    SELECT later.id
    FROM sources later
    JOIN sources first ON first.name = later.name AND first.id < later.id
);

CREATE UNIQUE INDEX sources_name_unique ON sources (name);
