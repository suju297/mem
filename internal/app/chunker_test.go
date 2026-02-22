package app

import (
	"testing"

	memtoken "mempack/internal/token"
)

func testTokenCounter(t *testing.T) *memtoken.Counter {
	t.Helper()
	counter, err := memtoken.New("cl100k_base")
	if err != nil {
		t.Fatalf("create tokenizer: %v", err)
	}
	return counter
}

func TestChunkFilePythonSemantic(t *testing.T) {
	content := []byte(`@trace
def top(value):
    return value

class Greeter:
    def hello(self, name):
        return f"hello {name}"

async def worker():
    return 1
`)
	counter := testTokenCounter(t)
	chunks, err := chunkFile("sample.py", content, 400, 20, counter)
	if err != nil {
		t.Fatalf("chunk file: %v", err)
	}

	if len(chunks) != 3 {
		t.Fatalf("expected 3 semantic chunks, got %d", len(chunks))
	}

	byName := map[string]SemanticChunk{}
	for _, chunk := range chunks {
		byName[chunk.SymbolName] = chunk
	}

	top, ok := byName["top"]
	if !ok {
		t.Fatalf("missing top function chunk")
	}
	if top.SymbolKind != "function" || top.ChunkType != "function" {
		t.Fatalf("unexpected top metadata: kind=%s type=%s", top.SymbolKind, top.ChunkType)
	}
	if top.StartLine != 1 {
		t.Fatalf("expected decorator to be included in top chunk, start_line=%d", top.StartLine)
	}

	greeter, ok := byName["Greeter"]
	if !ok {
		t.Fatalf("missing Greeter class chunk")
	}
	if greeter.SymbolKind != "class" || greeter.ChunkType != "class" {
		t.Fatalf("unexpected Greeter metadata: kind=%s type=%s", greeter.SymbolKind, greeter.ChunkType)
	}

	worker, ok := byName["worker"]
	if !ok {
		t.Fatalf("missing worker function chunk")
	}
	if worker.SymbolKind != "function" || worker.ChunkType != "function" {
		t.Fatalf("unexpected worker metadata: kind=%s type=%s", worker.SymbolKind, worker.ChunkType)
	}
}

func TestChunkFileTypeScriptSemantic(t *testing.T) {
	content := []byte(`export interface User {
  id: string;
}

@sealed
export class Service {
  run() {
    return 1;
  }
}

export function makeUser(name: string): User {
  return { id: name };
}

const handler = async () => {
  return "ok";
};
`)
	counter := testTokenCounter(t)
	chunks, err := chunkFile("sample.ts", content, 400, 20, counter)
	if err != nil {
		t.Fatalf("chunk file: %v", err)
	}

	if len(chunks) != 4 {
		t.Fatalf("expected 4 semantic chunks, got %d", len(chunks))
	}

	byName := map[string]SemanticChunk{}
	for _, chunk := range chunks {
		byName[chunk.SymbolName] = chunk
	}

	user, ok := byName["User"]
	if !ok {
		t.Fatalf("missing User interface chunk")
	}
	if user.SymbolKind != "interface" || user.ChunkType != "interface" {
		t.Fatalf("unexpected User metadata: kind=%s type=%s", user.SymbolKind, user.ChunkType)
	}

	service, ok := byName["Service"]
	if !ok {
		t.Fatalf("missing Service class chunk")
	}
	if service.SymbolKind != "class" || service.ChunkType != "class" {
		t.Fatalf("unexpected Service metadata: kind=%s type=%s", service.SymbolKind, service.ChunkType)
	}
	if service.StartLine != 5 {
		t.Fatalf("expected decorator to be included in class chunk, start_line=%d", service.StartLine)
	}

	makeUser, ok := byName["makeUser"]
	if !ok {
		t.Fatalf("missing makeUser function chunk")
	}
	if makeUser.SymbolKind != "function" || makeUser.ChunkType != "function" {
		t.Fatalf("unexpected makeUser metadata: kind=%s type=%s", makeUser.SymbolKind, makeUser.ChunkType)
	}

	handler, ok := byName["handler"]
	if !ok {
		t.Fatalf("missing handler function chunk")
	}
	if handler.SymbolKind != "function" || handler.ChunkType != "function" {
		t.Fatalf("unexpected handler metadata: kind=%s type=%s", handler.SymbolKind, handler.ChunkType)
	}
}

func TestChunkFileTypeScriptFallsBackToLineChunks(t *testing.T) {
	content := []byte(`console.log("hello")
console.log("world")
`)
	counter := testTokenCounter(t)
	chunks, err := chunkFile("plain.ts", content, 400, 20, counter)
	if err != nil {
		t.Fatalf("chunk file: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 fallback chunk, got %d", len(chunks))
	}
	if chunks[0].ChunkType != "line" {
		t.Fatalf("expected line chunk, got %s", chunks[0].ChunkType)
	}
	if chunks[0].SymbolName != "" || chunks[0].SymbolKind != "" {
		t.Fatalf("expected empty symbol metadata for fallback chunk, got name=%q kind=%q", chunks[0].SymbolName, chunks[0].SymbolKind)
	}
}

func TestChunkFileTypeScriptSemanticMultilineArrow(t *testing.T) {
	content := []byte(`const compute =
  async (
    value: number
  ) => {
    return value + 1;
  };
`)
	counter := testTokenCounter(t)
	chunks, err := chunkFile("multiline.ts", content, 400, 20, counter)
	if err != nil {
		t.Fatalf("chunk file: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 semantic chunk, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.SymbolName != "compute" || chunk.SymbolKind != "function" || chunk.ChunkType != "function" {
		t.Fatalf("unexpected chunk metadata: name=%q kind=%q type=%q", chunk.SymbolName, chunk.SymbolKind, chunk.ChunkType)
	}
}

func TestChunkFilePythonSemanticMultilineDecorator(t *testing.T) {
	content := []byte(`@app.route(
    "/api/users",
    methods=["GET"],
)
def get_users():
    return []
`)
	counter := testTokenCounter(t)
	chunks, err := chunkFile("routes.py", content, 400, 20, counter)
	if err != nil {
		t.Fatalf("chunk file: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 semantic chunk, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.SymbolName != "get_users" || chunk.SymbolKind != "function" || chunk.ChunkType != "function" {
		t.Fatalf("unexpected chunk metadata: name=%q kind=%q type=%q", chunk.SymbolName, chunk.SymbolKind, chunk.ChunkType)
	}
	if chunk.StartLine != 1 {
		t.Fatalf("expected decorator block included, start_line=%d", chunk.StartLine)
	}
	if chunk.EndLine < 6 {
		t.Fatalf("expected full function block, end_line=%d", chunk.EndLine)
	}
}

func TestChunkFilePythonSemanticDecoratorColumnZeroArgument(t *testing.T) {
	content := []byte(`@deco(
some_global_value
)
def target():
    return 1
`)
	counter := testTokenCounter(t)
	chunks, err := chunkFile("odd_decorator.py", content, 400, 20, counter)
	if err != nil {
		t.Fatalf("chunk file: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 semantic chunk, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.SymbolName != "target" || chunk.SymbolKind != "function" || chunk.ChunkType != "function" {
		t.Fatalf("unexpected chunk metadata: name=%q kind=%q type=%q", chunk.SymbolName, chunk.SymbolKind, chunk.ChunkType)
	}
	if chunk.StartLine != 1 {
		t.Fatalf("expected decorator block included, start_line=%d", chunk.StartLine)
	}
}

func TestChunkFileTypeScriptSemanticMultilineDecorator(t *testing.T) {
	content := []byte(`@Component({
  selector: "app-root",
})
export class AppComponent {
  run() {
    return 1;
  }
}
`)
	counter := testTokenCounter(t)
	chunks, err := chunkFile("component.ts", content, 400, 20, counter)
	if err != nil {
		t.Fatalf("chunk file: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 semantic chunk, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.SymbolName != "AppComponent" || chunk.SymbolKind != "class" || chunk.ChunkType != "class" {
		t.Fatalf("unexpected chunk metadata: name=%q kind=%q type=%q", chunk.SymbolName, chunk.SymbolKind, chunk.ChunkType)
	}
	if chunk.StartLine != 1 {
		t.Fatalf("expected decorator block included, start_line=%d", chunk.StartLine)
	}
}

func TestChunkFileTypeScriptSemanticTypedConstFunction(t *testing.T) {
	content := []byte(`const handler: RequestHandler = async (req, res) => {
  return "ok";
};
`)
	counter := testTokenCounter(t)
	chunks, err := chunkFile("typed.ts", content, 400, 20, counter)
	if err != nil {
		t.Fatalf("chunk file: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 semantic chunk, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.SymbolName != "handler" || chunk.SymbolKind != "function" || chunk.ChunkType != "function" {
		t.Fatalf("unexpected chunk metadata: name=%q kind=%q type=%q", chunk.SymbolName, chunk.SymbolKind, chunk.ChunkType)
	}
}

func TestChunkFileJavaScriptSemantic(t *testing.T) {
	content := []byte(`const handler = (req, res) => {
  return "ok";
};
`)
	counter := testTokenCounter(t)
	chunks, err := chunkFile("handler.js", content, 400, 20, counter)
	if err != nil {
		t.Fatalf("chunk file: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 semantic chunk, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.SymbolName != "handler" || chunk.SymbolKind != "function" || chunk.ChunkType != "function" {
		t.Fatalf("unexpected chunk metadata: name=%q kind=%q type=%q", chunk.SymbolName, chunk.SymbolKind, chunk.ChunkType)
	}
}
