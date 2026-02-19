CREATE TRIGGER IF NOT EXISTS memories_ai
AFTER INSERT ON memories
WHEN NEW.deleted_at IS NULL
BEGIN
    INSERT INTO memories_fts (rowid, title, summary, tags, entities, repo_id, workspace, mem_id)
    VALUES (NEW.rowid, NEW.title, NEW.summary, COALESCE(NEW.tags_text, ''), COALESCE(NEW.entities_text, ''), NEW.repo_id, NEW.workspace, NEW.id);
END;

CREATE TRIGGER IF NOT EXISTS memories_au
AFTER UPDATE ON memories
BEGIN
    DELETE FROM memories_fts WHERE rowid = OLD.rowid;
    INSERT INTO memories_fts (rowid, title, summary, tags, entities, repo_id, workspace, mem_id)
    SELECT NEW.rowid, NEW.title, NEW.summary, COALESCE(NEW.tags_text, ''), COALESCE(NEW.entities_text, ''), NEW.repo_id, NEW.workspace, NEW.id
    WHERE NEW.deleted_at IS NULL;
END;

CREATE TRIGGER IF NOT EXISTS memories_ad
AFTER DELETE ON memories
BEGIN
    DELETE FROM memories_fts WHERE rowid = OLD.rowid;
END;

CREATE TRIGGER IF NOT EXISTS chunks_ai
AFTER INSERT ON chunks
WHEN NEW.deleted_at IS NULL
BEGIN
    INSERT INTO chunks_fts (rowid, locator, text, tags, repo_id, workspace, chunk_id, thread_id)
    VALUES (NEW.rowid, NEW.locator, NEW.text, COALESCE(NEW.tags_text, ''), NEW.repo_id, NEW.workspace, NEW.chunk_id, NEW.thread_id);
END;

CREATE TRIGGER IF NOT EXISTS chunks_au
AFTER UPDATE ON chunks
BEGIN
    DELETE FROM chunks_fts WHERE rowid = OLD.rowid;
    INSERT INTO chunks_fts (rowid, locator, text, tags, repo_id, workspace, chunk_id, thread_id)
    SELECT NEW.rowid, NEW.locator, NEW.text, COALESCE(NEW.tags_text, ''), NEW.repo_id, NEW.workspace, NEW.chunk_id, NEW.thread_id
    WHERE NEW.deleted_at IS NULL;
END;

CREATE TRIGGER IF NOT EXISTS chunks_ad
AFTER DELETE ON chunks
BEGIN
    DELETE FROM chunks_fts WHERE rowid = OLD.rowid;
END;
