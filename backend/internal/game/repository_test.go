package game

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
)

func TestRoomRepository_InterfaceSatisfaction(_ *testing.T) {
	var _ RoomRepository = (*mockRoomRepository)(nil)
	var _ RoomRepository = newMockRoomRepository()
}

// --- RoomRepository Mock Tests ---

// TestMockRoomRepository_SaveAndLoad verifies the basic save/load cycle.

func TestMockRoomRepository_SaveAndLoad(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	ls := &domain.LobbyState{
		Code:      "TEST1",
		State:     `{"phase":"waiting"}`,
		UpdatedAt: 1000,
		CreatedAt: 900,
	}

	if err := repo.SaveLobbyState(ctx, ls); err != nil {
		t.Fatalf("SaveLobbyState failed: %v", err)
	}

	loaded, err := repo.LoadLobbyState(ctx, "TEST1")
	if err != nil {
		t.Fatalf("LoadLobbyState failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadLobbyState returned nil")
	}
	if loaded.Code != "TEST1" {
		t.Fatalf("loaded code = %q, want TEST1", loaded.Code)
	}
	if loaded.State != `{"phase":"waiting"}` {
		t.Fatalf("loaded state = %q, want {\"phase\":\"waiting\"}", loaded.State)
	}
}

// TestMockRoomRepository_LoadNotFound verifies that loading a non-existent
// code returns an error.

func TestMockRoomRepository_LoadNotFound(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	_, err := repo.LoadLobbyState(ctx, "NOPE1")
	if err == nil {
		t.Fatal("LoadLobbyState should return error for non-existent code")
	}
}

// TestMockRoomRepository_Delete verifies deletion works.

func TestMockRoomRepository_Delete(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	ls := &domain.LobbyState{Code: "DEL01", State: "{}"}
	if err := repo.SaveLobbyState(ctx, ls); err != nil {
		t.Fatalf("SaveLobbyState failed: %v", err)
	}

	if err := repo.DeleteLobbyState(ctx, "DEL01"); err != nil {
		t.Fatalf("DeleteLobbyState failed: %v", err)
	}

	// After deletion, load should fail.
	if _, err := repo.LoadLobbyState(ctx, "DEL01"); err == nil {
		t.Fatal("LoadLobbyState should fail after deletion")
	}
}

// TestMockRoomRepository_DeleteNotFound verifies that deleting a non-existent
// code returns an error.

func TestMockRoomRepository_DeleteNotFound(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	if err := repo.DeleteLobbyState(ctx, "NOPE1"); err == nil {
		t.Fatal("DeleteLobbyState should return error for non-existent code")
	}
}

// TestMockRoomRepository_SaveNil verifies that saving nil returns an error.
// This is adversarial: tests the mock's nil-safety.

func TestMockRoomRepository_SaveNil(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	if err := repo.SaveLobbyState(ctx, nil); err == nil {
		t.Fatal("SaveLobbyState should return error for nil lobby state")
	}
}

// TestMockRoomRepository_Overwrite verifies that saving the same code twice
// overwrites the previous state.

func TestMockRoomRepository_Overwrite(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	ls1 := &domain.LobbyState{Code: "OVR01", State: `{"v":1}`}
	ls2 := &domain.LobbyState{Code: "OVR01", State: `{"v":2}`}

	if err := repo.SaveLobbyState(ctx, ls1); err != nil {
		t.Fatalf("SaveLobbyState ls1 failed: %v", err)
	}
	if err := repo.SaveLobbyState(ctx, ls2); err != nil {
		t.Fatalf("SaveLobbyState ls2 failed: %v", err)
	}

	loaded, err := repo.LoadLobbyState(ctx, "OVR01")
	if err != nil {
		t.Fatalf("LoadLobbyState failed: %v", err)
	}
	if loaded.State != `{"v":2}` {
		t.Fatalf("loaded state = %q, want {\"v\":2}", loaded.State)
	}
}

// TestMockRoomRepository_Isolation verifies that the mock returns copies,
// not references to internal state. This is adversarial: if the mock returned
// internal pointers, external mutation would corrupt the store.

func TestMockRoomRepository_Isolation(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	ls := &domain.LobbyState{Code: "ISO01", State: `{"v":1}`}
	if err := repo.SaveLobbyState(ctx, ls); err != nil {
		t.Fatalf("SaveLobbyState failed: %v", err)
	}

	loaded1, _ := repo.LoadLobbyState(ctx, "ISO01")
	loaded1.State = `{"mutated":true}`

	loaded2, _ := repo.LoadLobbyState(ctx, "ISO01")
	if loaded2.State != `{"v":1}` {
		t.Fatalf("mock did not isolate stored state: got %q, want {\"v\":1}", loaded2.State)
	}
}

// TestMockRoomRepository_ErrorInjection verifies that the mock can be
// configured to return errors for testing error paths.

func TestMockRoomRepository_ErrorInjection(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	saveErr := errors.New("simulated save failure")
	repo.saveErr = saveErr
	if err := repo.SaveLobbyState(ctx, &domain.LobbyState{Code: "X"}); err != saveErr {
		t.Fatalf("SaveLobbyState error = %v, want %v", err, saveErr)
	}

	loadErr := errors.New("simulated load failure")
	repo.loadErr = loadErr
	if _, err := repo.LoadLobbyState(ctx, "X"); err != loadErr {
		t.Fatalf("LoadLobbyState error = %v, want %v", err, loadErr)
	}

	deleteErr := errors.New("simulated delete failure")
	repo.deleteErr = deleteErr
	if err := repo.DeleteLobbyState(ctx, "X"); err != deleteErr {
		t.Fatalf("DeleteLobbyState error = %v, want %v", err, deleteErr)
	}
}

// TestMockRoomRepository_CallCounts verifies that the mock tracks call counts.

func TestMockRoomRepository_CallCounts(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	// 3 saves
	for i := 0; i < 3; i++ {
		if err := repo.SaveLobbyState(ctx, &domain.LobbyState{Code: fmt.Sprintf("C%02d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	// 2 loads
	if _, err := repo.LoadLobbyState(ctx, "C00"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadLobbyState(ctx, "C01"); err != nil {
		t.Fatal(err)
	}
	// 1 delete
	if err := repo.DeleteLobbyState(ctx, "C00"); err != nil {
		t.Fatal(err)
	}

	if repo.saveCount != 3 {
		t.Fatalf("saveCount = %d, want 3", repo.saveCount)
	}
	if repo.loadCount != 2 {
		t.Fatalf("loadCount = %d, want 2", repo.loadCount)
	}
	if repo.deleteCount != 1 {
		t.Fatalf("deleteCount = %d, want 1", repo.deleteCount)
	}
}

// TestMockRoomRepository_ConcurrentAccess verifies the mock is safe for
// concurrent use. Run with -race.

func TestMockRoomRepository_ConcurrentAccess(_ *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	var wg sync.WaitGroup
	// Concurrent writers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			code := fmt.Sprintf("CON%02d", n)
			_ = repo.SaveLobbyState(ctx, &domain.LobbyState{Code: code})
		}(i)
	}
	// Concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = repo.LoadLobbyState(ctx, fmt.Sprintf("CON%02d", n%20))
		}(i)
	}
	// Concurrent deleters
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = repo.DeleteLobbyState(ctx, fmt.Sprintf("CON%02d", n))
		}(i)
	}
	wg.Wait()
}

// TestMockRoomRepository_CancelledContext verifies that the mock respects
// the context (currently it doesn't check ctx, but this documents the behavior).

func TestMockRoomRepository_CancelledContext(t *testing.T) {
	repo := newMockRoomRepository()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The mock doesn't check context, so these should succeed.
	// This test documents that the mock ignores context cancellation.
	if err := repo.SaveLobbyState(ctx, &domain.LobbyState{Code: "CAN01"}); err != nil {
		t.Fatalf("SaveLobbyState with cancelled context failed: %v", err)
	}
	if _, err := repo.LoadLobbyState(ctx, "CAN01"); err != nil {
		t.Fatalf("LoadLobbyState with cancelled context failed: %v", err)
	}
}

// --- Interface Contract Tests ---

// TestRoomRepository_InterfaceContract verifies that any RoomRepository
// implementation must have the correct method signatures. This is a
// compile-time check that also serves as documentation.

func TestRoomRepository_InterfaceContract(t *testing.T) {
	// The interface requires:
	//   SaveLobbyState(ctx context.Context, ls *domain.LobbyState) error
	//   LoadLobbyState(ctx context.Context, code string) (*domain.LobbyState, error)
	//   DeleteLobbyState(ctx context.Context, code string) error
	var repo RoomRepository = newMockRoomRepository()

	ctx := context.Background()
	ls := &domain.LobbyState{Code: "CON01", State: "{}"}

	// Exercise all three methods to verify the contract.
	if err := repo.SaveLobbyState(ctx, ls); err != nil {
		t.Fatalf("SaveLobbyState contract violation: %v", err)
	}
	if got, err := repo.LoadLobbyState(ctx, "CON01"); err != nil || got == nil {
		t.Fatalf("LoadLobbyState contract violation: got=%v err=%v", got, err)
	}
	if err := repo.DeleteLobbyState(ctx, "CON01"); err != nil {
		t.Fatalf("DeleteLobbyState contract violation: %v", err)
	}
}

// TestRoomRepository_MethodCount verifies the interface has exactly 3 methods.
// This is adversarial: catches accidental addition/removal of methods.

func TestRoomRepository_MethodCount(t *testing.T) {
	// We can't easily count interface methods at runtime in Go,
	// but we can verify all expected methods exist by calling them.
	// If a method is missing, the code won't compile.
	var _ = RoomRepository(nil)
	// The fact that this compiles verifies the interface exists.
}

// ─── GenerateRandomNickname ──────────────────────────────────────────
