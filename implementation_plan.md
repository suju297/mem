# Implementation Plan: Optimize Tokenizer Initialization

## Goal
Eliminate the ~50-90ms tokenizer initialization cost during `mem get` by strictly enforcing lazy loading. The tokenizer should ONLY be initialized if:
1. Retrieved items (state, memories, chunks) lack cached token counts.
2. The user explicitly requests debugging/token details that require re-computation.

## Current State Analysis
- `get.go` currently initializes the tokenizer if `applyBudget` returns `ErrTokenizerRequired`.
- `applyBudget` likely returns this error because one or more items (State, Memories, or Chunks) have `0` token counts.
- `state_tokens` column exists in DB but might not be populated or updated correctly.
- Memories/Chunks have `summary_tokens`/`text_tokens` columns.

## Proposed Changes

### 1. Identify Missing Counts
I need to ensure ALL paths that add data populate the token counts.
- **State**: `SetStateCurrent` and `AddStateHistory` need to ensure `state_tokens` is calculated.
- **Memories**: `AddMemory` handles it, but verify.
- **Chunks**: `Ingest` handles it, but verify.

### 2. Update `get.go` Logic
- The current logic in `get.go` is already lazy: `if errors.Is(err, ErrTokenizerRequired)`.
- If this block is being hit in benchmarks, it means we ARE missing counts.
- I need to debug *which* item is missing counts.

### 3. Ensure State Token Caching
- When `mem checkpoint` or state updates happen, `state_tokens` must be calculated and stored.
- `loadState` in `state.go` should return `stateTokens`.

## Execution Steps
1.  **Debug**: run `mem get --debug` and verify if `TokenizerInit` time is non-zero. If so, find out *why* budget required it.
2.  **State**: Check `internal/app/state.go` and `internal/store/records.go`. Ensure `state_tokens` is read/written.
3.  **Fallback**: Implement "approximate counting" (`chars/4`) as a fallback if desired? No, exact counts preferred, but ensure they are cached.
4.  **Verification**: Run `mem get` on a populated repo. Even after restart, it should NOT initialize tokenizer.

## Verification Plan
- Run `mem get "token"` on the existing test repo.
- Verify `tokenizer_init` is ~0.00ms (or not present) in debug output.
- Verify total latency drops by ~70ms.
