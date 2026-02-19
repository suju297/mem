export interface DoctorReport {
  ok: boolean;
  error?: string;
  suggestion?: string;
  repo?: {
    id?: string;
    git_root?: string;
  };
  db?: {
    path?: string;
    exists?: boolean;
    size_bytes?: number;
  };
  schema?: {
    user_version?: number;
    current_version?: number;
    last_migration_at?: string;
  };
  fts?: {
    memories?: boolean;
    chunks?: boolean;
    rebuilt?: boolean;
  };
  state?: {
    valid?: boolean;
    invalid_workspaces?: string[];
    repaired?: boolean;
  };
}

export interface ThreadItem {
  thread_id: string;
  title?: string;
  tags_json?: string;
  created_at: string;
  memory_count: number;
}

export interface ThreadMemoryBrief {
  id: string;
  title: string;
  summary: string;
  created_at: string;
  anchor_commit?: string;
  superseded_by?: string;
}

export interface ThreadShowResponse {
  thread: ThreadItem;
  memories: ThreadMemoryBrief[];
}

export interface RecentMemoryItem {
  id: string;
  thread_id: string;
  title: string;
  summary: string;
  created_at: string;
  anchor_commit?: string;
  superseded_by?: string;
}

export interface SessionItem {
  id: string;
  thread_id: string;
  title: string;
  summary: string;
  created_at: string;
  anchor_commit?: string;
  superseded_by?: string;
}

export interface SessionCount {
  count: number;
}

export interface AddMemoryResponse {
  id: string;
  thread_id: string;
  thread_used?: string;
  thread_defaulted?: boolean;
  title: string;
  anchor_commit?: string;
  created_at?: string;
}

export interface UpdateMemoryResponse {
  id: string;
  thread_id: string;
  title: string;
  summary: string;
  updated_at?: string;
}

export interface MemoryDetail {
  id: string;
  repo_id: string;
  thread_id?: string;
  title: string;
  summary: string;
  tags_json?: string;
  entities_json?: string;
  created_at: string;
  anchor_commit?: string;
  superseded_by?: string;
  deleted_at?: string;
}

export interface ChunkDetail {
  id: string;
  repo_id: string;
  artifact_id?: string;
  thread_id?: string;
  locator?: string;
  text: string;
  tags_json?: string;
  created_at: string;
  deleted_at?: string;
}

export interface ShowResponse {
  kind: "memory" | "chunk";
  memory?: MemoryDetail;
  chunk?: ChunkDetail;
}

export interface EmbedStatusResponse {
  repo_id: string;
  workspace: string;
  provider: string;
  model?: string;
  enabled: boolean;
  error?: string;
  note?: string;
  vectors: {
    provider_configured: string;
    model_configured?: string;
    configured: boolean;
    available: boolean;
    enabled: boolean;
    reason?: string;
    how_to_fix?: string[];
    effective_provider?: string;
    effective_model?: string;
  };
}

export interface ContextPack {
  version: string;
  tool: string;
  repo: {
    repo_id: string;
    git_root: string;
    head?: string;
    branch?: string;
  };
  workspace: string;
  search_meta?: SearchMeta;
  state: any;
  matched_threads: MatchedThread[];
  top_memories: ContextMemoryItem[];
  top_chunks: ContextChunkItem[];
  top_chunks_raw?: ContextChunkItem[];
  link_trail: LinkTrail[];
  rules: string[];
  budget: BudgetInfo;
}

export interface SearchMeta {
  query?: string;
  sanitized_query?: string;
  mode: string;
  mode_used: string;
  vector_used: boolean;
  rewritten_query?: string;
  rewrites_applied?: string[];
  fallback_reason?: string;
  warnings?: string[];
  intent?: string;
  entities_found?: number;
  time_hint?: string;
  recency_boost?: number;
  clusters_formed?: number;
}

export interface BudgetInfo {
  tokenizer: string;
  target_total: number;
  used_total: number;
}

export interface MatchedThread {
  thread_id: string;
  why: string;
}

export interface ContextMemoryItem {
  id: string;
  thread_id?: string;
  title: string;
  summary: string;
  anchor_commit?: string;
  links?: string[];
  is_cluster?: boolean;
  cluster_size?: number;
  cluster_ids?: string[];
  similarity?: number;
}

export interface ContextChunkItem {
  chunk_id: string;
  artifact_id?: string;
  thread_id?: string;
  locator?: string;
  text: string;
  sources?: ChunkSource[];
}

export interface ChunkSource {
  chunk_id: string;
  artifact_id?: string;
  thread_id?: string;
  locator?: string;
  created_at?: string;
}

export interface LinkTrail {
  from: string;
  rel: string;
  to: string;
}
