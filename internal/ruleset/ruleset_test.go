package ruleset

import "testing"

func TestSplitMix64Stable(t *testing.T) {
	r := NewRNG(1001)
	want := []int{36, 4, 97, 19}
	for i, expected := range want {
		if got := r.Index(100); got != expected {
			t.Fatalf("decision %d: got %d, want %d", i, got, expected)
		}
	}
	if r.Consumed() != 4 {
		t.Fatalf("consumed=%d", r.Consumed())
	}
}

func TestDefaultDependencyStable(t *testing.T) {
	d := DefaultDependency()
	if d.ID != DefaultID || len(d.ContentHash) != len("sha256:")+64 {
		t.Fatalf("bad default dependency: %#v", d)
	}
	if d.ExecutionBudget.Instructions == 0 || d.ExecutionBudget.ContinuationBytes == 0 {
		t.Fatalf("default dependency has no execution budget: %#v", d.ExecutionBudget)
	}
}

func TestRNGSnapshotCloneAndRestore(t *testing.T) {
	r := NewRNG(42)
	r.Index(10)
	snapshot := r.Snapshot()
	clone := r.Clone()
	if got, want := clone.Index(100), r.Index(100); got != want {
		t.Fatalf("clone diverged: got %d, want %d", got, want)
	}
	r.Restore(snapshot)
	if r.Snapshot() != snapshot {
		t.Fatalf("restore failed: got %#v, want %#v", r.Snapshot(), snapshot)
	}
}
