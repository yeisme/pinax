package operation

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// TestConcurrentCLIAndMCPStoreAccess 模拟 `pinax mcp` server 与另一个 CLI
// 进程各自打开同一 vault 操作账本（两个独立连接池）：并行读 Get 与
// CreateAccepted/终态迁移写入，全程无 StoreUnavailable / SQLITE_BUSY。
// 这是 pinax-local-async-substrate-v1 统一 DSN 的 store 级验收。
func TestConcurrentCLIAndMCPStoreAccess(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mcpStore, err := Open(root)
	if err != nil {
		t.Fatalf("Open(mcp side) error = %v", err)
	}
	defer func() { _ = mcpStore.Close() }()
	cliStore, err := Open(root)
	if err != nil {
		t.Fatalf("Open(cli side) error = %v", err)
	}
	defer func() { _ = cliStore.Close() }()

	ctx := context.Background()
	seed, err := mcpStore.CreateAccepted(ctx, testCreateRequest(t, "op-seed-0001", "seed-key-0001", map[string]string{"seed": "1"}))
	if err != nil {
		t.Fatalf("seed CreateAccepted() error = %v", err)
	}
	seedID := seed.Operation.OperationID

	const writeCount = 30
	const readerCount = 3

	stop := make(chan struct{})
	var readerErrsMu sync.Mutex
	var readerErrs []error

	// MCP 侧：并行读者持续读取既有 operation。
	var readersWG sync.WaitGroup
	for i := 0; i < readerCount; i++ {
		readersWG.Add(1)
		go func() {
			defer readersWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := mcpStore.Get(ctx, seedID); err != nil {
					readerErrsMu.Lock()
					readerErrs = append(readerErrs, err)
					readerErrsMu.Unlock()
					return
				}
			}
		}()
	}

	// CLI 侧：写者循环 CreateAccepted + 终态迁移；MCP 侧穿插少量写。
	for i := 0; i < writeCount; i++ {
		index := fmt.Sprintf("%04d", i)
		result, err := cliStore.CreateAccepted(ctx, testCreateRequest(t, "op-conc-"+index, "conc-key-"+index, map[string]int{"i": i}))
		if err != nil {
			close(stop)
			t.Fatalf("CreateAccepted(%d) error = %v", i, err)
		}
		if _, err := cliStore.StartApplying(ctx, result.Operation.OperationID); err != nil {
			close(stop)
			t.Fatalf("StartApplying(%d) error = %v", i, err)
		}
		if _, err := cliStore.CompleteSucceeded(ctx, result.Operation.OperationID, Outcome{}); err != nil {
			close(stop)
			t.Fatalf("CompleteSucceeded(%d) error = %v", i, err)
		}
		if i%5 == 0 {
			if _, err := mcpStore.CreateAccepted(ctx, testCreateRequest(t, "op-mcp-"+index, "mcp-key-"+index, map[string]int{"i": i})); err != nil {
				close(stop)
				t.Fatalf("mcp-side CreateAccepted(%d) error = %v", i, err)
			}
		}
	}
	close(stop)
	readersWG.Wait()

	readerErrsMu.Lock()
	defer readerErrsMu.Unlock()
	for _, err := range readerErrs {
		t.Fatalf("parallel reader error: %v", err)
	}

	// 双侧写入总量可观测且无丢失：seed + writeCount 基础项 + 每 5 项一个 MCP 侧项。
	var total int64
	if err := mcpStore.db.WithContext(ctx).Model(&OperationRow{}).Count(&total).Error; err != nil {
		t.Fatalf("final count error = %v", err)
	}
	want := int64(1 + writeCount + writeCount/5)
	if total != want {
		t.Fatalf("total operations = %d, want %d", total, want)
	}
}
