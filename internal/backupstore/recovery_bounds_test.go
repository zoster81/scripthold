package backupstore

import "testing"

func TestRecoveryBoundsReusePersistentStorePolicy(t *testing.T) {
	defaults := DefaultRecoveryBounds()
	if defaults.MaxManifests != defaultMaxManifests || defaults.MaxObjects != defaultMaxManifests || defaults.MaxBytes != defaultMaxTotalBytes {
		t.Fatalf("recovery defaults=%#v", defaults)
	}
	got, err := NormalizeRecoveryBounds(RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if got != defaults {
		t.Fatalf("normalized defaults=%#v want=%#v", got, defaults)
	}

	for _, bounds := range []RecoveryBounds{
		{MaxManifests: hardMaxManifests + 1},
		{MaxObjects: hardMaxManifests + 1},
		{MaxBytes: hardMaxTotalBytes + 1},
	} {
		if _, err := NormalizeRecoveryBounds(bounds); err == nil {
			t.Fatalf("out-of-range recovery bounds accepted: %#v", bounds)
		}
	}
}
