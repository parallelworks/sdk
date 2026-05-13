package parallelworks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdentityCredential(t *testing.T) {
	tests := []struct {
		name     string
		identity Identity
		want     string
	}{
		{"api key", Identity{ApiKey: "pwt_abc.def"}, "pwt_abc.def"},
		{"token", Identity{Token: "header.payload.sig"}, "header.payload.sig"},
		{"both prefers api key", Identity{ApiKey: "pwt_abc.def", Token: "tok"}, "pwt_abc.def"},
		{"empty", Identity{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.identity.Credential(); got != tt.want {
				t.Errorf("Credential() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIdentityUsername(t *testing.T) {
	tests := []struct {
		name          string
		canonicalName string
		want          string
	}{
		{"standard", "user:alice@cloud.parallel.works", "alice"},
		{"no prefix", "alice@cloud.parallel.works", "alice"},
		{"empty", "", ""},
		{"no at sign", "user:alice", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := Identity{CanonicalName: tt.canonicalName}
			if got := id.Username(); got != tt.want {
				t.Errorf("Username() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadCredentialConfigFrom_NotFound(t *testing.T) {
	cfg, err := LoadCredentialConfigFrom("/nonexistent/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Identities) != 0 {
		t.Errorf("expected empty identities, got %d", len(cfg.Identities))
	}
}

func TestLoadCredentialConfigFrom_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")
	os.WriteFile(path, []byte(""), 0600)

	cfg, err := LoadCredentialConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Identities) != 0 {
		t.Errorf("expected empty identities, got %d", len(cfg.Identities))
	}
}

func TestLoadCredentialConfigFrom_WhitespaceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")
	os.WriteFile(path, []byte("   \n\t\n  "), 0600)

	cfg, err := LoadCredentialConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Identities) != 0 {
		t.Errorf("expected empty identities, got %d", len(cfg.Identities))
	}
}

func TestLoadCredentialConfigFrom_Unparseable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")

	for _, payload := range []string{
		"not valid json{{{",
		`{"identi`,
		`{"identities":{"user:foo":{"token":"eyJhbG`,
	} {
		if err := os.WriteFile(path, []byte(payload), 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		cfg, err := LoadCredentialConfigFrom(path)
		if err != nil {
			t.Fatalf("payload %q: unexpected error: %v", payload, err)
		}
		if len(cfg.Identities) != 0 {
			t.Errorf("payload %q: expected empty identities, got %d", payload, len(cfg.Identities))
		}
	}
}

func TestSaveTo_Atomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")

	cfg := &CredentialConfig{
		Identities: map[string]Identity{
			"dev": {Server: "cloud.parallel.works", Name: "dev", ApiKey: "pwt_abc.def"},
		},
		CurrentIdentity: "dev",
	}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != ".credentials" {
			t.Errorf("leftover file in dir: %q", e.Name())
		}
	}

	loaded, err := LoadCredentialConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadCredentialConfigFrom: %v", err)
	}
	if loaded.CurrentIdentity != "dev" || loaded.Identities["dev"].ApiKey != "pwt_abc.def" {
		t.Errorf("round-trip mismatch: %+v", loaded)
	}
}

func TestLoadCredentialConfigFrom_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")
	data := `{
		"identities": {
			"dev": {
				"apikey": "pwt_abc.def",
				"server": "cloud.parallel.works",
				"name": "dev",
				"canonicalName": "user:alice@cloud.parallel.works",
				"organization": "myorg"
			}
		},
		"currentIdentity": "dev"
	}`
	os.WriteFile(path, []byte(data), 0600)

	cfg, err := LoadCredentialConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CurrentIdentity != "dev" {
		t.Errorf("CurrentIdentity = %q, want %q", cfg.CurrentIdentity, "dev")
	}
	id, exists := cfg.Identities["dev"]
	if !exists {
		t.Fatal("expected identity 'dev' to exist")
	}
	if id.Organization != "myorg" {
		t.Errorf("Organization = %q, want %q", id.Organization, "myorg")
	}
	if id.Username() != "alice" {
		t.Errorf("Username() = %q, want %q", id.Username(), "alice")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pw", ".credentials")

	cfg := &CredentialConfig{
		Identities: map[string]Identity{
			"test": {
				ApiKey:        "pwt_abc.def",
				Server:        "example.com",
				Name:          "test",
				CanonicalName: "user:bob@example.com",
				Organization:  "org1",
			},
		},
		CurrentIdentity: "test",
	}

	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo failed: %v", err)
	}

	loaded, err := LoadCredentialConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadCredentialConfigFrom failed: %v", err)
	}
	if loaded.CurrentIdentity != "test" {
		t.Errorf("CurrentIdentity = %q, want %q", loaded.CurrentIdentity, "test")
	}
	if loaded.Identities["test"].ApiKey != "pwt_abc.def" {
		t.Errorf("ApiKey = %q, want %q", loaded.Identities["test"].ApiKey, "pwt_abc.def")
	}
}

func TestResolveIdentity_FromCredentialConfig(t *testing.T) {
	cfg := &CredentialConfig{
		Identities: map[string]Identity{
			"dev":  {ApiKey: "pwt_abc.def", Server: "dev.example.com", Organization: "org1"},
			"prod": {Token: "h.p.s", Server: "prod.example.com", Organization: "org2"},
		},
		CurrentIdentity: "dev",
	}

	t.Run("default context", func(t *testing.T) {
		id, err := cfg.ResolveIdentity()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.ApiKey != "pwt_abc.def" {
			t.Errorf("got ApiKey %q, want %q", id.ApiKey, "pwt_abc.def")
		}
	})

	t.Run("context override", func(t *testing.T) {
		id, err := cfg.ResolveIdentity(WithContext("prod"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.Token != "h.p.s" {
			t.Errorf("got Token %q, want %q", id.Token, "h.p.s")
		}
	})

	t.Run("platform host override", func(t *testing.T) {
		id, err := cfg.ResolveIdentity(WithPlatformHost("custom.example.com"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.Server != "custom.example.com" {
			t.Errorf("got Server %q, want %q", id.Server, "custom.example.com")
		}
	})

	t.Run("context not found", func(t *testing.T) {
		_, err := cfg.ResolveIdentity(WithContext("nonexistent"))
		if err == nil {
			t.Fatal("expected error for nonexistent context")
		}
	})

	t.Run("no context configured", func(t *testing.T) {
		empty := &CredentialConfig{Identities: map[string]Identity{}}
		_, err := empty.ResolveIdentity()
		if err != ErrNoContextConfigured {
			t.Errorf("got %v, want ErrNoContextConfigured", err)
		}
	})
}

func TestResolveIdentity_EnvVar(t *testing.T) {
	cfg := &CredentialConfig{
		Identities: map[string]Identity{
			"dev":  {ApiKey: "pwt_dev.key", Server: "dev.example.com"},
			"prod": {ApiKey: "pwt_prod.key", Server: "prod.example.com"},
		},
		CurrentIdentity: "dev",
	}

	t.Setenv("PW_CONTEXT", "prod")

	id, err := cfg.ResolveIdentity()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.ApiKey != "pwt_prod.key" {
		t.Errorf("PW_CONTEXT should select prod, got ApiKey %q", id.ApiKey)
	}
}

func TestResolveIdentity_FlagOverridesEnvVar(t *testing.T) {
	cfg := &CredentialConfig{
		Identities: map[string]Identity{
			"dev":  {ApiKey: "pwt_dev.key", Server: "dev.example.com"},
			"prod": {ApiKey: "pwt_prod.key", Server: "prod.example.com"},
		},
		CurrentIdentity: "dev",
	}

	t.Setenv("PW_CONTEXT", "prod")

	id, err := cfg.ResolveIdentity(WithContext("dev"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.ApiKey != "pwt_dev.key" {
		t.Errorf("WithContext should override PW_CONTEXT, got ApiKey %q", id.ApiKey)
	}
}

func TestContexts(t *testing.T) {
	cfg := &CredentialConfig{
		Identities: map[string]Identity{
			"beta":  {Server: "beta.example.com", Organization: "org1"},
			"alpha": {Server: "alpha.example.com", Organization: "org2"},
		},
		CurrentIdentity: "alpha",
	}

	contexts := cfg.Contexts()
	if len(contexts) != 2 {
		t.Fatalf("expected 2 contexts, got %d", len(contexts))
	}
	// Should be sorted alphabetically
	if contexts[0].Name != "alpha" {
		t.Errorf("first context should be alpha, got %q", contexts[0].Name)
	}
	if !contexts[0].IsCurrent {
		t.Error("alpha should be current")
	}
	if contexts[1].IsCurrent {
		t.Error("beta should not be current")
	}
}

func TestContextNames(t *testing.T) {
	cfg := &CredentialConfig{
		Identities: map[string]Identity{
			"beta":  {},
			"alpha": {},
		},
	}
	names := cfg.ContextNames()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("got %v, want [alpha beta]", names)
	}
}

func TestSetCurrentContext(t *testing.T) {
	cfg := &CredentialConfig{
		Identities:      map[string]Identity{"dev": {}, "prod": {}},
		CurrentIdentity: "dev",
	}

	if err := cfg.SetCurrentContext("prod"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CurrentIdentity != "prod" {
		t.Errorf("CurrentIdentity = %q, want %q", cfg.CurrentIdentity, "prod")
	}

	if err := cfg.SetCurrentContext("nonexistent"); err != ErrContextNotFound {
		t.Errorf("got %v, want ErrContextNotFound", err)
	}

	if err := cfg.SetCurrentContext(""); err != ErrEmptyContextName {
		t.Errorf("got %v, want ErrEmptyContextName", err)
	}
}

func TestRenameContext(t *testing.T) {
	cfg := &CredentialConfig{
		Identities: map[string]Identity{
			"old-name": {Server: "example.com", Name: "old-name", CanonicalName: "user:alice@example.com"},
		},
		CurrentIdentity: "old-name",
	}

	if err := cfg.RenameContext("old-name", "new-name"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := cfg.Identities["old-name"]; exists {
		t.Error("old name should be removed")
	}
	if _, exists := cfg.Identities["new-name"]; !exists {
		t.Error("new name should exist")
	}
	if cfg.CurrentIdentity != "new-name" {
		t.Errorf("CurrentIdentity = %q, want %q", cfg.CurrentIdentity, "new-name")
	}
	if cfg.Identities["new-name"].Name != "new-name" {
		t.Errorf("Name = %q, want %q", cfg.Identities["new-name"].Name, "new-name")
	}
}

func TestRenameContext_SetsCanonicalNameWhenEmpty(t *testing.T) {
	cfg := &CredentialConfig{
		Identities: map[string]Identity{
			"legacy": {Server: "example.com"},
		},
		CurrentIdentity: "legacy",
	}

	if err := cfg.RenameContext("legacy", "renamed"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Identities["renamed"].CanonicalName != "legacy" {
		t.Errorf("CanonicalName = %q, want %q", cfg.Identities["renamed"].CanonicalName, "legacy")
	}
}

func TestDeleteContext(t *testing.T) {
	t.Run("delete non-current", func(t *testing.T) {
		cfg := &CredentialConfig{
			Identities:      map[string]Identity{"dev": {}, "prod": {}},
			CurrentIdentity: "dev",
		}
		newCurrent, err := cfg.DeleteContext("prod", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newCurrent != "" {
			t.Errorf("newCurrent = %q, want empty", newCurrent)
		}
		if _, exists := cfg.Identities["prod"]; exists {
			t.Error("prod should be deleted")
		}
	})

	t.Run("delete current without force", func(t *testing.T) {
		cfg := &CredentialConfig{
			Identities:      map[string]Identity{"dev": {}},
			CurrentIdentity: "dev",
		}
		_, err := cfg.DeleteContext("dev", false)
		if err != ErrCannotDeleteCurrent {
			t.Errorf("got %v, want ErrCannotDeleteCurrent", err)
		}
	})

	t.Run("delete current with force", func(t *testing.T) {
		cfg := &CredentialConfig{
			Identities:      map[string]Identity{"alpha": {}, "beta": {}},
			CurrentIdentity: "beta",
		}
		newCurrent, err := cfg.DeleteContext("beta", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newCurrent != "alpha" {
			t.Errorf("newCurrent = %q, want %q", newCurrent, "alpha")
		}
	})

	t.Run("delete last context", func(t *testing.T) {
		cfg := &CredentialConfig{
			Identities:      map[string]Identity{"only": {}},
			CurrentIdentity: "only",
		}
		newCurrent, err := cfg.DeleteContext("only", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newCurrent != "" {
			t.Errorf("newCurrent = %q, want empty", newCurrent)
		}
	})
}

func TestUpsertContext(t *testing.T) {
	t.Run("create new with explicit name", func(t *testing.T) {
		cfg := &CredentialConfig{Identities: map[string]Identity{}}
		err := cfg.UpsertContext("myctx", "user:alice@example.com", "example.com", "org1", "pwt_abc.def")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		id := cfg.Identities["myctx"]
		if id.ApiKey != "pwt_abc.def" {
			t.Errorf("ApiKey = %q, want %q", id.ApiKey, "pwt_abc.def")
		}
		if cfg.CurrentIdentity != "myctx" {
			t.Errorf("CurrentIdentity = %q, want %q", cfg.CurrentIdentity, "myctx")
		}
	})

	t.Run("create new without explicit name", func(t *testing.T) {
		cfg := &CredentialConfig{Identities: map[string]Identity{}}
		err := cfg.UpsertContext("", "user:alice@example.com", "example.com", "org1", "h.p.s")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		id := cfg.Identities["user:alice@example.com"]
		if id.Token != "h.p.s" {
			t.Errorf("Token = %q, want %q", id.Token, "h.p.s")
		}
	})

	t.Run("update existing by canonical name", func(t *testing.T) {
		cfg := &CredentialConfig{
			Identities: map[string]Identity{
				"renamed": {
					ApiKey:        "old-key",
					Server:        "example.com",
					CanonicalName: "user:alice@example.com",
					Organization:  "org1",
				},
			},
		}
		err := cfg.UpsertContext("", "user:alice@example.com", "example.com", "org1", "pwt_new.key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Identities["renamed"].ApiKey != "pwt_new.key" {
			t.Errorf("ApiKey should be updated, got %q", cfg.Identities["renamed"].ApiKey)
		}
	})

	t.Run("multiple contexts same canonical name errors", func(t *testing.T) {
		cfg := &CredentialConfig{
			Identities: map[string]Identity{
				"ctx1": {CanonicalName: "user:alice@example.com"},
				"ctx2": {CanonicalName: "user:alice@example.com"},
			},
		}
		err := cfg.UpsertContext("", "user:alice@example.com", "example.com", "org1", "pwt_abc.def")
		if err != ErrMultipleContexts {
			t.Errorf("got %v, want ErrMultipleContexts", err)
		}
	})
}

func TestPlatformHost(t *testing.T) {
	client := NewClient("https://cloud.parallel.works")
	if got := client.PlatformHost(); got != "cloud.parallel.works" {
		t.Errorf("PlatformHost() = %q, want %q", got, "cloud.parallel.works")
	}

	client2 := NewClient("http://localhost:8080")
	if got := client2.PlatformHost(); got != "localhost:8080" {
		t.Errorf("PlatformHost() = %q, want %q", got, "localhost:8080")
	}
}
