package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/yoann/kern-orch/internal/graph"
)

func TestAnonymizePIIMasksIbanAndEmailButKeepsOrdinaryText(t *testing.T) {
	s := graph.NewState()
	s.Set("extracted_text", "Salaire net 2400 euros. IBAN FR7630006000011234567890189. Contact: jean.dupont@example.com.")

	if err := anonymizePII(context.Background(), s); err != nil {
		t.Fatalf("anonymizePII: %v", err)
	}

	masked, ok := s.Get("masked_text")
	if !ok {
		t.Fatal("masked_text not set")
	}
	maskedText := masked.(string)

	if strings.Contains(maskedText, "FR7630006000011234567890189") {
		t.Errorf("masked_text still contains the raw IBAN: %q", maskedText)
	}
	if strings.Contains(maskedText, "jean.dupont@example.com") {
		t.Errorf("masked_text still contains the raw email: %q", maskedText)
	}
	if !strings.Contains(maskedText, "Salaire net 2400 euros") {
		t.Errorf("masked_text lost ordinary text: %q", maskedText)
	}

	tm, ok := s.Get("pii_token_map")
	if !ok {
		t.Fatal("pii_token_map not set")
	}
	tokenMap := tm.(map[string]string)
	if len(tokenMap) != 2 {
		t.Fatalf("expected 2 masked entities, got %d: %#v", len(tokenMap), tokenMap)
	}
	var sawIban, sawEmail bool
	for token, original := range tokenMap {
		if original == "FR7630006000011234567890189" {
			sawIban = true
			if !strings.Contains(maskedText, token) {
				t.Errorf("masked_text does not contain the IBAN's own token %q", token)
			}
		}
		if original == "jean.dupont@example.com" {
			sawEmail = true
		}
	}
	if !sawIban || !sawEmail {
		t.Errorf("token map missing an entity: %#v", tokenMap)
	}
}

func TestAnonymizePIIErrorsWhenExtractedTextMissing(t *testing.T) {
	s := graph.NewState()

	if err := anonymizePII(context.Background(), s); err == nil {
		t.Fatal("expected an error when extracted_text is absent")
	}
}

func TestAnonymizePIIIsIdempotentOnTextWithNoPII(t *testing.T) {
	s := graph.NewState()
	s.Set("extracted_text", "Bonjour, ceci est un texte sans donnée sensible.")

	if err := anonymizePII(context.Background(), s); err != nil {
		t.Fatalf("anonymizePII: %v", err)
	}

	masked, _ := s.Get("masked_text")
	if masked.(string) != "Bonjour, ceci est un texte sans donnée sensible." {
		t.Errorf("text with no PII should pass through unchanged, got %q", masked)
	}
	tm, _ := s.Get("pii_token_map")
	if len(tm.(map[string]string)) != 0 {
		t.Errorf("expected an empty token map, got %#v", tm)
	}
}

func TestDeanonymizePIIRestoresOriginalValues(t *testing.T) {
	s := graph.NewState()
	s.Set("interpretation_masked", `{"iban":"<IBAN_1>","email":"<EMAIL_1>"}`)
	s.Set("pii_token_map", map[string]string{
		"<IBAN_1>":  "FR7630006000011234567890189",
		"<EMAIL_1>": "jean.dupont@example.com",
	})

	if err := deanonymizePII(context.Background(), s); err != nil {
		t.Fatalf("deanonymizePII: %v", err)
	}

	result, ok := s.Get("interpretation")
	if !ok {
		t.Fatal("interpretation not set")
	}
	want := `{"iban":"FR7630006000011234567890189","email":"jean.dupont@example.com"}`
	if result.(string) != want {
		t.Errorf("got %q, want %q", result.(string), want)
	}
}

// A checkpoint round-trip serializes state through JSON, so a map[string]string set
// before a Freeze/persist comes back as map[string]interface{} after reload — this must
// not break restoration.
func TestDeanonymizePIIHandlesTokenMapAfterJSONRoundTrip(t *testing.T) {
	s := graph.NewState()
	s.Set("interpretation_masked", `Le titulaire est <PII_1>.`)
	s.Set("pii_token_map", map[string]interface{}{
		"<PII_1>": "Jean Dupont",
	})

	if err := deanonymizePII(context.Background(), s); err != nil {
		t.Fatalf("deanonymizePII: %v", err)
	}

	result, _ := s.Get("interpretation")
	if result.(string) != "Le titulaire est Jean Dupont." {
		t.Errorf("got %q", result.(string))
	}
}

func TestDeanonymizePIIIsSafeWithoutATokenMap(t *testing.T) {
	s := graph.NewState()
	s.Set("interpretation_masked", "Rien à démasquer ici.")

	if err := deanonymizePII(context.Background(), s); err != nil {
		t.Fatalf("deanonymizePII: %v", err)
	}

	result, _ := s.Get("interpretation")
	if result.(string) != "Rien à démasquer ici." {
		t.Errorf("got %q", result.(string))
	}
}
