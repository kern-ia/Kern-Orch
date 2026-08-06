---
name: courtage-extraction
type: agent
description: Extrait les données structurées d'un dossier de rachat de crédit, rédige un draft de mémorandum de financement, et notifie l'équipe des pièces manquantes — OCR pur, masquage PII avant tout modèle de raisonnement, validation humaine sur les étapes qui comptent
graph: examples/courtage-extraction.yaml
---

# courtage-extraction

Besoins #1 (extraction documentaire), #2 (copilote de mémorandum) et #3 (relances pièces
manquantes) de l'agence de courtage (rachat de crédit), enchaînés dans le MÊME graphe/run
— voir `Kern-Orch/skills/specs.md`, section 2, pour le cadrage complet.

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

## Besoin #2 — Copilote de mémorandum

Enchaîné sur le besoin #1, dans le MÊME run (choix explicite : "un seul flux
dossier → mémo" — kern-orch n'a pas de mécanisme de partage d'état entre deux runs
distincts, donc chaîner pour de vrai veut dire rester dans le même graphe).

```
extraction_validee → memo_prep → masquage_memo → redaction_memo → demasquage_memo → confirm_memo
```

- **memo_prep** (`agent_cli.py`) : assemble le dossier extrait (`interpretation`, déjà
  démasqué) et les notes du premier entretien (`notes_entretien`) avant un nouveau
  masquage. `notes_entretien` DOIT être fourni par un `nudge` sur le run
  (`POST /api/v1/runs/{id}/nudge {"key":"notes_entretien","value":"..."}`) pendant la
  pause à `confirm_extraction` — c'est le moment naturel où l'analyste ajoute son contexte
  avant de valider l'extraction et de laisser le graphe continuer. Erreur claire si absent
  (même discipline anti-invention que partout ailleurs : pas d'historique client inventé).
- **masquage_memo** / **demasquage_memo** (tools Go) : même discipline PII que le besoin
  #1, mais avec ses propres clés d'état (`memo_text`/`memo_masked_text`/`memo_token_map`,
  `memo_draft_masked`/`memo_draft`) — ne touche jamais `extracted_text`/`masked_text`/
  `interpretation` du besoin #1, qui doivent survivre dans l'état final aux côtés du
  mémorandum (l'analyste a besoin des deux).
- **redaction_memo** (`agent_cli.py`, `claude -p`) : rédige un draft de Mémorandum de
  Financement structuré (situation patrimoniale, besoin, analyse revenus/charges avec
  statuts honnêtes, points de vigilance, recommandation préliminaire non définitive).
- **confirm_memo** : validation humaine du draft — "l'analyste n'a plus qu'à relire et
  affiner" (besoin prospect original).

## Besoin #3 — Relances pièces manquantes

En parallèle du mémorandum (pas séquentiel), déclenché juste après `extraction_validee` :

```
extraction_validee → [onRelanceNeeded] → relance_prep → relance_notify
                                       ↘ relance_non_necessaire (si rien ne manque)
```

- **onRelanceNeeded** (`internal/cmd/courtage_relance.go`) : lit `pieces_manquantes` dans
  `interpretation` (le dossier démasqué du besoin #1), route toujours vers `memo_prep` en
  plus de la branche relance — un seul routeur combine les deux car kern-orch n'autorise
  qu'une route par nœud source.
- **relance_prep** (`agent_cli.py`) : formate `state["message"]` (la clé fixe que lit le
  tool `notify` déjà construit) à partir de la vraie liste de pièces manquantes. Ne
  fabrique jamais une liste si le dossier est illisible — message générique demandant une
  vérification manuelle à la place.
- **relance_notify** (tool `notify`, déjà construit) : envoie sur le canal Telegram
  interne déjà configuré (`KERN_TELEGRAM_BOT_TOKEN`/`KERN_TELEGRAM_CHAT_ID`).
- **Notifie l'équipe interne, jamais le client** : décision cadrée avant de coder — aucun
  modèle client → coordonnées n'existe, et Telegram ne peut pas contacter un utilisateur
  qui n'a pas parlé au bot en premier. L'IA rédige, l'humain transmet.
- **Pas de validation humaine avant l'envoi** — notification interne, pas une action
  visible du client (contrairement à la publication de `community-management-agency`).

## Reste à faire

Voir `Kern-Orch/skills/specs.md` section 2 : ingestion UI/Telegram, vérification réelle
du chemin Mistral OCR, détection de noms propres (nécessiterait un moteur NLP dans
kern-anon), pas d'édition du draft de mémorandum dans une UI (comme le calendrier
marketing de Kern-UI), et le besoin #4 (RAG banques, bloqué sur `kern-memory`).
