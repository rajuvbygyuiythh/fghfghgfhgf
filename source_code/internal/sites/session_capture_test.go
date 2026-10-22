package sites

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrompt10SessionCaptureValidateClearAndIsolation(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := EnsureSites(dataDir); err != nil {
		t.Fatalf("EnsureSites: %v", err)
	}

	// Initially missing
	initSum, err := ValidateSiteSession(dataDir, "omezy")
	if err != nil {
		t.Fatalf("ValidateSiteSession(omezy): %v", err)
	}
	if initSum.Status != SessionStateMissing {
		t.Fatalf("expected initial status missing, got %s", initSum.Status)
	}

	// Capture session for omezy
	capSum, err := CaptureSiteSession(dataDir, "omezy", nil)
	if err != nil {
		t.Fatalf("CaptureSiteSession(omezy): %v", err)
	}
	if capSum.Status != SessionStateValid || capSum.CookieCount == 0 || !capSum.HasAuthToken {
		t.Fatalf("unexpected captured summary: %+v", capSum)
	}

	// Verify all 5 required files exist inside sites/omezy/sessions/default/
	ctxOmezy, err := NewSiteContext(dataDir, "omezy")
	if err != nil {
		t.Fatalf("NewSiteContext(omezy): %v", err)
	}
	for _, fn := range []string{"cookies.json", "local_storage.json", "ws_endpoints.json", "auth_tokens.json", "meta.json"} {
		rel := filepath.Join("sessions", "default", fn)
		info, statErr := ctxOmezy.Stat(rel)
		if statErr != nil || info.Size() == 0 {
			t.Fatalf("expected non-empty %s in sites/omezy/sessions/default/, err=%v", fn, statErr)
		}
	}

	// Verify another site (chatib) was NOT touched by omezy's session capture
	chatibSum, err := ValidateSiteSession(dataDir, "chatib")
	if err != nil {
		t.Fatalf("ValidateSiteSession(chatib): %v", err)
	}
	if chatibSum.Status != SessionStateMissing {
		t.Fatalf("expected chatib session to remain missing, got %+v", chatibSum)
	}

	// Capture chatib as well, then simulate omezy expiring in 2 hours
	if _, err := CaptureSiteSession(dataDir, "chatib", nil); err != nil {
		t.Fatalf("CaptureSiteSession(chatib): %v", err)
	}
	bundle, ok, err := LoadCapturedSessionBundle(dataDir, "omezy")
	if err != nil || !ok {
		t.Fatalf("LoadCapturedSessionBundle(omezy): ok=%v err=%v", ok, err)
	}
	bundle.Meta.ExpiresAt = time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	if err := ctxOmezy.SaveJSON(filepath.Join("sessions", "default", "meta.json"), bundle.Meta); err != nil {
		t.Fatalf("SaveJSON(meta.json): %v", err)
	}
	expSum, err := ValidateSiteSession(dataDir, "omezy")
	if err != nil || expSum.Status != SessionStateExpiring {
		t.Fatalf("expected omezy status=expiring, got %+v (err=%v)", expSum, err)
	}

	// GetAllSessionStatuses must report 1 valid (chatib), 1 expiring (omezy), and the rest missing
	overview, err := GetAllSessionStatuses(dataDir)
	if err != nil {
		t.Fatalf("GetAllSessionStatuses: %v", err)
	}
	if overview.Valid != 1 || overview.Expiring != 1 || overview.Missing != overview.Total-2 {
		t.Fatalf("unexpected overview counts: total=%d valid=%d expiring=%d missing=%d",
			overview.Total, overview.Valid, overview.Expiring, overview.Missing)
	}

	// Clear omezy session; verify omezy is missing while chatib remains valid
	clrSum, err := ClearSiteSession(dataDir, "omezy")
	if err != nil || clrSum.Status != SessionStateMissing {
		t.Fatalf("ClearSiteSession(omezy): %+v (err=%v)", clrSum, err)
	}
	if _, statErr := ctxOmezy.Stat(filepath.Join("sessions", "default", "meta.json")); !os.IsNotExist(statErr) {
		t.Fatalf("expected sites/omezy/sessions/default/meta.json removed after ClearSiteSession")
	}
	chatibAfter, err := ValidateSiteSession(dataDir, "chatib")
	if err != nil || chatibAfter.Status != SessionStateValid {
		t.Fatalf("expected chatib session untouched after clearing omezy, got %+v", chatibAfter)
	}
}
