package store

import "database/sql"

func scanMemoryFields(scan func(dest ...any) error) (Memory, error) {
	var mem Memory
	var createdAt string
	var deletedAt sql.NullString
	var threadID sql.NullString
	var summaryTokens sql.NullInt64
	var tagsJSON sql.NullString
	var tagsText sql.NullString
	var entitiesJSON sql.NullString
	var entitiesText sql.NullString
	var anchorCommit sql.NullString
	var supersededBy sql.NullString
	if err := scan(
		&mem.ID,
		&mem.RepoID,
		&mem.Workspace,
		&threadID,
		&mem.Title,
		&mem.Summary,
		&summaryTokens,
		&tagsJSON,
		&tagsText,
		&entitiesJSON,
		&entitiesText,
		&createdAt,
		&anchorCommit,
		&supersededBy,
		&deletedAt,
	); err != nil {
		return Memory{}, err
	}
	mem.ThreadID = threadID.String
	if summaryTokens.Valid {
		mem.SummaryTokens = int(summaryTokens.Int64)
	}
	mem.TagsJSON = tagsJSON.String
	mem.TagsText = tagsText.String
	mem.EntitiesJSON = entitiesJSON.String
	mem.EntitiesText = entitiesText.String
	mem.AnchorCommit = anchorCommit.String
	mem.SupersededBy = supersededBy.String
	mem.CreatedAt = parseTime(createdAt)
	if deletedAt.Valid {
		mem.DeletedAt = parseTime(deletedAt.String)
	}
	return mem, nil
}

func scanChunkFields(scan func(dest ...any) error) (Chunk, error) {
	var chunk Chunk
	var createdAt string
	var deletedAt sql.NullString
	var artifactID sql.NullString
	var threadID sql.NullString
	var locator sql.NullString
	var text sql.NullString
	var textHash sql.NullString
	var textTokens sql.NullInt64
	var tagsJSON sql.NullString
	var tagsText sql.NullString
	var chunkType sql.NullString
	var symbolName sql.NullString
	var symbolKind sql.NullString
	if err := scan(
		&chunk.ID,
		&chunk.RepoID,
		&chunk.Workspace,
		&artifactID,
		&threadID,
		&locator,
		&text,
		&textHash,
		&textTokens,
		&tagsJSON,
		&tagsText,
		&chunkType,
		&symbolName,
		&symbolKind,
		&createdAt,
		&deletedAt,
	); err != nil {
		return Chunk{}, err
	}
	chunk.ArtifactID = artifactID.String
	chunk.ThreadID = threadID.String
	chunk.Locator = locator.String
	chunk.Text = text.String
	chunk.TextHash = textHash.String
	if textTokens.Valid {
		chunk.TextTokens = int(textTokens.Int64)
	}
	chunk.TagsJSON = tagsJSON.String
	chunk.TagsText = tagsText.String
	chunk.ChunkType = chunkType.String
	chunk.SymbolName = symbolName.String
	chunk.SymbolKind = symbolKind.String
	chunk.CreatedAt = parseTime(createdAt)
	if deletedAt.Valid {
		chunk.DeletedAt = parseTime(deletedAt.String)
	}
	return chunk, nil
}

func scanMemorySearchFields(scan func(dest ...any) error) (Memory, error) {
	var mem Memory
	var createdAt string
	var threadID sql.NullString
	var summaryTokens sql.NullInt64
	var tagsJSON sql.NullString
	var entitiesJSON sql.NullString
	var anchorCommit sql.NullString
	var supersededBy sql.NullString
	if err := scan(
		&mem.ID,
		&mem.RepoID,
		&mem.Workspace,
		&threadID,
		&mem.Title,
		&mem.Summary,
		&summaryTokens,
		&tagsJSON,
		&entitiesJSON,
		&createdAt,
		&anchorCommit,
		&supersededBy,
	); err != nil {
		return Memory{}, err
	}
	mem.ThreadID = threadID.String
	if summaryTokens.Valid {
		mem.SummaryTokens = int(summaryTokens.Int64)
	}
	mem.TagsJSON = tagsJSON.String
	mem.EntitiesJSON = entitiesJSON.String
	mem.AnchorCommit = anchorCommit.String
	mem.SupersededBy = supersededBy.String
	mem.CreatedAt = parseTime(createdAt)
	return mem, nil
}

func scanChunkSearchFields(scan func(dest ...any) error) (Chunk, error) {
	var chunk Chunk
	var createdAt string
	var artifactID sql.NullString
	var threadID sql.NullString
	var locator sql.NullString
	var text sql.NullString
	var textHash sql.NullString
	var textTokens sql.NullInt64
	var tagsJSON sql.NullString
	var tagsText sql.NullString
	var chunkType sql.NullString
	var symbolName sql.NullString
	var symbolKind sql.NullString
	if err := scan(
		&chunk.ID,
		&chunk.RepoID,
		&chunk.Workspace,
		&artifactID,
		&threadID,
		&locator,
		&text,
		&textHash,
		&textTokens,
		&tagsJSON,
		&tagsText,
		&chunkType,
		&symbolName,
		&symbolKind,
		&createdAt,
	); err != nil {
		return Chunk{}, err
	}
	chunk.ArtifactID = artifactID.String
	chunk.ThreadID = threadID.String
	chunk.Locator = locator.String
	chunk.Text = text.String
	chunk.TextHash = textHash.String
	if textTokens.Valid {
		chunk.TextTokens = int(textTokens.Int64)
	}
	chunk.TagsJSON = tagsJSON.String
	chunk.TagsText = tagsText.String
	chunk.ChunkType = chunkType.String
	chunk.SymbolName = symbolName.String
	chunk.SymbolKind = symbolKind.String
	chunk.CreatedAt = parseTime(createdAt)
	return chunk, nil
}
