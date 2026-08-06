---
name: courtage-extraction
type: agent
description: Extrait les données structurées (revenus, crédits en cours, reste à vivre, pièces manquantes) d'un dossier de rachat de crédit — OCR pur, masquage PII avant tout modèle de raisonnement, validation humaine du résultat final
graph: examples/courtage-extraction.yaml
---

# courtage-extraction

Premier besoin de l'agence de courtage (rachat de crédit) — voir
`Kern-Orch/skills/specs.md`, section 2, pour le cadrage complet (découpage global,
raisonnement sur l'ordre des étapes, format de sortie).

Dispatché depuis le chat (`/courtage-extraction /chemin/vers/dossier.pdf`) : le texte du
message EST le chemin du document (`mailbox.Nudge("message", text)` côté kern-orch,
mécanisme identique à `community-management-agency`). C'est le seul canal d'ingestion
câblé pour l'instant — upload UI et réception Telegram sont des besoins séparés, non
construits, consignés dans `specs.md`.

## Pipeline

```
reception → extraction → masquage_pii → interpretation → demasquage_pii → confirm_extraction
```

- **reception** (`agent_cli.py`) : vérifie que le document existe.
- **extraction** (`agent_cli.py`) : découpe le PDF page par page (boucle interne, pas de
  structure de graphe — kern-orch n'a pas de boucle native), texte de la couche PDF en
  priorité, OCR seulement sur les pages sans couche texte (une image scannée). OCR pur —
  jamais un modèle qui "comprend" le document, pour ne rien envoyer en clair à un tiers
  avant le masquage.
- **masquage_pii** (`internal/cmd/courtage_anon.go`, tool Go, `kern-anon`) : remplace les
  entités PII détectées par des jetons séquentiels (`<IBAN_1>`, `<EMAIL_1>`, `<PII_1>`
  pour tout ce qui n'a pas d'étiquette dédiée) plutôt que le `Deanonymize` positionnel de
  kern-anon — une sortie JSON reformulée par un modèle ne préserve pas les offsets de
  caractères d'origine, la substitution par jeton si.
- **interpretation** (`agent_cli.py`, `claude -p`) : lit le texte masqué, produit un JSON
  structuré (revenus, crédits en cours, incidents, reste à vivre, pièces manquantes),
  chaque enregistrement avec sa source et un statut honnête (`"confirmé"` /
  `"à vérifier"`) — jamais un chiffre inventé.
- **demasquage_pii** (tool Go) : restitue les valeurs réelles dans le JSON final, par
  substitution de chaîne sur la carte jeton → original.
- **confirm_extraction** : validation humaine du résultat final — la seule étape où les
  données réelles redeviennent visibles, par un humain qui doit de toute façon les
  vérifier, jamais envoyées en clair ailleurs.

## OCR : deux moteurs, switchables

Même patron que `send_telegram`/`send_x` dans `community-management-agency` : vide =
repli sûr, configuré = vrai appel.

- **Local par défaut** (Tesseract, `pytesseract`) — fonctionne sans rien configurer.
  Nécessite `brew install tesseract tesseract-lang` et `pip install pytesseract pymupdf
  pillow` dans le venv utilisé par `run.sh`.
- **Mistral OCR** dès que `MISTRAL_API_KEY` est présent dans l'environnement — fournisseur
  EU. **Non vérifié en conditions réelles à ce stade** (pas de clé disponible au moment de
  construire ce skill) : le code existe et est testé (appel HTTP mocké), mais aucun appel
  réel n'a encore été fait. À vérifier dès qu'une clé est disponible.

## Limite connue : pas de détection de nom de personne

`kern-anon` détecte les entités par motif (IBAN, IBAN, téléphone FR, email, NIR,
SIREN/SIRET, carte bancaire...) — aucun moteur NLP n'est branché (pas de modèle ONNX
chargé), donc un nom propre en texte libre ("Jean Dupont") n'est PAS masqué. Les champs
structurés à motif fixe (coordonnées bancaires, identifiants, contacts) le sont. À
documenter clairement au client : ce n'est pas un masquage complet de toute PII, c'est un
masquage des données à motif reconnaissable.

## Reste à faire

Voir `Kern-Orch/skills/specs.md` section 2 : ingestion UI/Telegram, vérification réelle
du chemin Mistral OCR, détection de noms propres (nécessiterait un moteur NLP dans
kern-anon), et les besoins #2 à #4 (mémorandum, relances, RAG banques).
