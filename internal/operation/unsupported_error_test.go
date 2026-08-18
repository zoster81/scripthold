package operation

import "testing"

func TestUnsupportedKindIsStable(t *testing.T) {
	err := New(KindUnsupported, "cross-filesystem move is unsupported")
	if got := KindOf(err); got != KindUnsupported {
		t.Fatalf("KindOf() = %v, want KindUnsupported", got)
	}
	if got := KindUnsupported.String(); got != "unsupported" {
		t.Fatalf("KindUnsupported.String() = %q, want unsupported", got)
	}
}
