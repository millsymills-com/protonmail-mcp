package session

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	gokeyring "github.com/zalando/go-keyring"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/keyring"
)

func TestKeyringCacheClearedByClearMethod(t *testing.T) {
	s := &Session{raw: newRawClient("http://invalid.test", nil)}
	s.keyrings = &keyring.Keyrings{} // pretend a prior unlock populated it
	s.clearKeyringCache()
	if s.keyrings != nil {
		t.Fatal("keyring cache must be nil after clearKeyringCache")
	}
}

func TestLogoutClearsKeyringCache(t *testing.T) {
	gokeyring.MockInit()
	s := &Session{kc: keychain.New(), raw: newRawClient("http://invalid.test", nil)}
	s.keyrings = &keyring.Keyrings{}
	if err := s.Logout(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if s.keyrings != nil {
		t.Fatal("Logout must clear the keyring cache")
	}
}

func TestKeyringsCacheHitSkipsClient(t *testing.T) {
	// When keyrings is already set, Keyrings(ctx) returns it without calling
	// Client (which would fail against "http://invalid.test").
	s := &Session{raw: newRawClient("http://invalid.test", nil)}
	want := &keyring.Keyrings{}
	s.keyrings = want

	got, err := s.Keyrings(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatal("Keyrings must return the cached value")
	}
}

// TestKeyringsSingleFlightReusesWinner drives the post-lock re-check. The winner
// enters the fetcher and parks there holding unlockMu; a second caller then
// reads the empty cache, queues on unlockMu, and — once the winner finishes —
// must reuse the winner's result via the re-check rather than unlock again. The
// fetcher counts invocations: dropping the re-check makes the second caller
// fetch a second time, so the count climbs to 2 and the test fails.
func TestKeyringsSingleFlightReusesWinner(t *testing.T) {
	var fetchCalls int32
	winnerIn := make(chan struct{})
	release := make(chan struct{})
	s := &Session{
		kc:  &credKC{creds: keychain.Creds{Password: "pw"}},
		raw: newRawClient("http://invalid.test", nil),
	}
	f := lockedFetcher(t, "pw")
	s.keyFetcher = func(context.Context) (keyring.KeyFetcher, error) {
		if atomic.AddInt32(&fetchCalls, 1) == 1 {
			close(winnerIn) // first caller is the winner, parked inside the unlock
			<-release
		}
		return f, nil
	}

	winnerErr := make(chan error, 1)
	go func() { _, err := s.Keyrings(context.Background()); winnerErr <- err }()
	<-winnerIn // winner now holds unlockMu inside the fetcher; cache still empty

	secondErr := make(chan error, 1)
	var secondGot *keyring.Keyrings
	go func() {
		krs, err := s.Keyrings(context.Background()) // reads empty cache, queues on unlockMu
		secondGot = krs
		secondErr <- err
	}()

	// Give the second caller time to park on unlockMu before the winner finishes.
	time.Sleep(20 * time.Millisecond)
	close(release)

	if err := <-winnerErr; err != nil {
		t.Fatalf("winner: %v", err)
	}
	if err := <-secondErr; err != nil {
		t.Fatalf("second caller: %v", err)
	}
	if secondGot != s.keyrings {
		t.Fatal("second caller must reuse the winner's cached keyrings")
	}
	if n := atomic.LoadInt32(&fetchCalls); n != 1 {
		t.Fatalf("fetcher ran %d times, want 1; the post-lock re-check is missing", n)
	}
}
