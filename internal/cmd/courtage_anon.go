package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/YoLaub/PresidioGo/analyzer"
	"github.com/YoLaub/PresidioGo/anonymizer"
	"github.com/YoLaub/PresidioGo/registry"
	"github.com/yoann/kern-orch/internal/graph"
)

// piiTokenLabels maps kern-anon's fr/generic recognizer entity types to the short,
// human-readable prefix used in the sequential mask token (<PREFIX_N>). Any entity type
// not listed here (a future recognizer this file wasn't updated for) still gets masked
// through the DEFAULT operator below with prefix "PII" — never left in clear, only less
// self-descriptive to the interpreting model.
var piiTokenLabels = map[string]string{
	"FR_NIR":           "NIR",
	"FR_SIREN":         "SIREN",
	"FR_SIRET":         "SIRET",
	"FR_LICENSE_PLATE": "PLAQUE",
	"FR_PHONE_NUMBER":  "TELEPHONE",
	"EMAIL_ADDRESS":    "EMAIL",
	"IBAN_CODE":        "IBAN",
	"CREDIT_CARD":      "CARTE",
	"CRYPTO":           "CRYPTO",
	"MAC_ADDRESS":      "MAC",
	"IP_ADDRESS":       "IP",
	"URL":              "URL",
}

// newMaskOperators builds one Custom operator per known entity type (plus a DEFAULT
// catch-all), each writing its masked value into a shared token map instead of Presidio's
// own position-based Deanonymize. A Claude-transformed JSON output cannot preserve the
// original text's rune offsets, so restoring PII by exact position (Deanonymize's
// contract) does not fit this pipeline — plain string substitution against this map does,
// regardless of how the text in between was rewritten.
func newMaskOperators() (map[string]anonymizer.Operator, map[string]string) {
	tokenMap := make(map[string]string)
	counters := make(map[string]int)

	mk := func(label string) anonymizer.Operator {
		return anonymizer.Custom("mask_token", func(v string) (string, error) {
			counters[label]++
			token := fmt.Sprintf("<%s_%d>", label, counters[label])
			tokenMap[token] = v
			return token, nil
		})
	}

	ops := make(map[string]anonymizer.Operator, len(piiTokenLabels)+1)
	for entityType, label := range piiTokenLabels {
		ops[entityType] = mk(label)
	}
	ops["DEFAULT"] = mk("PII")
	return ops, tokenMap
}

// anonymizePII masks PII in state key "extracted_text" (the pure-OCR output, before any
// interpretive model sees it — see specs.md "Besoin #1") and writes "masked_text" plus
// "pii_token_map" (token -> original value). Errors rather than proceeding on an absent
// input: a missing extracted_text means an upstream node didn't run, not that there is
// nothing to mask.
func anonymizePII(ctx context.Context, s *graph.State) error {
	raw, ok := s.Get("extracted_text")
	if !ok {
		return errors.New("courtage: extracted_text manquant")
	}
	text, _ := raw.(string)

	eng, err := analyzer.New(analyzer.WithRegistry(registry.Default("fr")))
	if err != nil {
		return fmt.Errorf("courtage: initialisation de l'analyseur PII : %w", err)
	}
	results, err := eng.Analyze(ctx, text, analyzer.Language("fr"))
	if err != nil {
		return fmt.Errorf("courtage: analyse PII : %w", err)
	}

	ops, tokenMap := newMaskOperators()
	result, err := anonymizer.New().Anonymize(text, results, ops)
	if err != nil {
		return fmt.Errorf("courtage: masquage PII : %w", err)
	}

	s.Set("masked_text", result.Text)
	s.Set("pii_token_map", tokenMap)
	return nil
}

// deanonymizePII restores original PII values into state key "interpretation_masked"
// (the model's masked-text interpretation) by plain string substitution against
// "pii_token_map", writing the result to "interpretation". A checkpoint round-trip
// serializes State through JSON, so a map[string]string set by anonymizePII before a
// Freeze/persist comes back as map[string]interface{} on reload — both shapes are
// accepted. A missing or empty token map is not an error: it means the extracted text
// had no PII to mask in the first place.
func deanonymizePII(_ context.Context, s *graph.State) error {
	raw, _ := s.Get("interpretation_masked")
	text, _ := raw.(string)

	tm, _ := s.Get("pii_token_map")
	for token, original := range asStringMap(tm) {
		text = strings.ReplaceAll(text, token, original)
	}

	s.Set("interpretation", text)
	return nil
}

func asStringMap(v any) map[string]string {
	switch m := v.(type) {
	case map[string]string:
		return m
	case map[string]any:
		out := make(map[string]string, len(m))
		for k, val := range m {
			if s, ok := val.(string); ok {
				out[k] = s
			}
		}
		return out
	default:
		return nil
	}
}
