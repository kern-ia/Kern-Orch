# Specs — agences Kern IA

Mémoire de travail : une catégorie par agence (skill/famille de skills), état réel et
reste à faire. Mis à jour à chaque session, pas un plan figé.

---

## 1. Agence Community Management (`community-management-agency`)

**Statut** : boucle centrale fonctionnelle et vérifiée en conditions réelles (pas un
concept) — 6 canaux, double mode stratège, deux vrais connecteurs de publication, mode
automatique séparé, tableau de bord calendrier éditable. Voir `docs/index/0021` à `0028`
(Kern-Orch) et les fiches équivalentes côté Kern-UI pour le détail vérifié de chaque
brique.

### Fait, vérifié en réel
- Canaux : LinkedIn, Instagram, X, TikTok (déclaratif), email (séquences multi-touches),
  newsletter/blog (format long), Telegram.
- Deux vrais connecteurs de publication : Telegram (Bot API), X (API v2, OAuth 1.0a signé
  à la main). Envoi réel confirmé (`message_id`/`tweet_id`) dans les deux cas.
- Double mode stratège : avis sur une stratégie fournie / proposition par le stratège
  (avec validation humaine uniquement dans ce second cas).
- Skill séparé `community-management-agency-auto` : saute la validation humaine
  uniquement sur un canal à vrai connecteur, jamais le comportement par défaut.
- Garde-fou anti-invention systématique (`[À COMPLÉTER]` plutôt qu'un chiffre inventé),
  vérifié sur du contenu réel à chaque canal.
- Tableau de bord "Marketing" (sous-onglet de Rédaction, Kern-UI) : calendrier grille,
  élément "Sans date" pour le contenu sans date extraite, panneau plein écran éditable +
  copie presse-papiers.

### Reste à faire (connu, pas juste "un jour")
- **Instagram / TikTok sans vrai connecteur** — contrainte de plateforme réelle : leurs
  API n'acceptent pas de texte seul, il faut une image/vidéo. Bloqué tant qu'une brique de
  génération d'image n'existe pas (piste : `kern-image`, ou skill appelant un service
  tiers type Higgsfield / GPT-image — aucun des deux construit).
- **Historique du calendrier marketing en mémoire seulement** — perdu au redémarrage de
  kern-ui. Pas de brique de stockage persistant construite cette session (choix explicite,
  pas un oubli).
- **Pas de republication depuis l'édition** — le panneau du calendrier permet de corriger
  et copier, jamais de renvoyer réellement le texte édité. Choix explicite de l'utilisateur
  (2026-08-06), à rouvrir si le besoin change.
- **Mode auto sans écran de confirmation dédié** — l'avertissement vit dans la fiche du
  skill (visible au moment de le choisir dans le Grimoire), pas dans une vraie modale de
  garde avant activation dans l'interface.
- **Détection de canal fragile aux variations de phrasé du modèle** — repose sur la ligne
  "Plateforme(s) :" du brief ; déjà cassée une fois en réel par un phrasé légèrement
  différent ("Plateforme :" sans le "(s)"), corrigée mais reste un point de fragilité
  générique (toute détection de contenu généré par un modèle doit être testée sur
  plusieurs phrasés réels, pas un seul exemple observé).

---

## 2. Agence de courtage (rachat de crédit)

**Statut** : besoins #1 (extraction documentaire) et #2 (copilote de mémorandum)
construits et vérifiés en conditions réelles, enchaînés dans le même skill/graphe — voir
`docs/index/0029-courtage-extraction.md` et `docs/index/0030-courtage-memorandum.md`.
Besoins #3/#4 pas commencés. Besoin source original : `agence_courtage.html` (ce dossier) —
spécification "Architecture & Workflows Agentiques : Rachat de Crédit", 4 swarms
d'agents, superviseur central, bus Pub/Sub, mémoire partagée chiffrée.

### Besoin #1 — Extraction documentaire : fait, vérifié en réel (2026-08-06)
- Skill `courtage-extraction` (`skills/courtage-extraction/`) : pipeline `reception →
  extraction → masquage_pii → interpretation → demasquage_pii → confirm_extraction`.
- `kern-anon` intégré comme vraie dépendance Go (`go.mod replace` vers le repo local) —
  premier vrai consommateur de la brique. Masquage par jeton séquentiel
  (`<IBAN_1>`, `<EMAIL_1>`, `<PII_1>` en repli), pas le `Deanonymize` positionnel natif —
  voir la fiche OKF pour le raisonnement.
- OCR : texte de la couche PDF en priorité, OCR (Tesseract local ou Mistral cloud selon
  `MISTRAL_API_KEY`) seulement sur une page sans couche texte. Chemin local vérifié en
  réel (Tesseract installé, PDF/image réels testés) ; chemin Mistral écrit et testé
  (mocké) mais **jamais appelé en vrai faute de clé API**.
- Deux vrais dispatch HTTP de bout en bout (texte natif + OCR, approbation + refus des
  deux côtés du routeur) — voir la fiche OKF pour le détail.

### Reste à faire — besoin #1
- **Vérifier le chemin Mistral OCR en conditions réelles** dès qu'une clé API est
  disponible (aujourd'hui : code écrit et testé, jamais exécuté pour de vrai).
- **Ingestion documentaire réelle** — seul le canal chemin/dossier est câblé (le texte du
  chat dispatch EST le chemin du fichier). Deux canaux voulus par l'utilisateur restent à
  construire : upload réel depuis l'UI (route multipart, chantier cross-repo kern-orch +
  kern-ui) et réception de documents via Telegram (le connecteur d'envoi existe déjà,
  recevoir des fichiers est du code nouveau : `getUpdates`/webhook + téléchargement via
  l'API Telegram).
- **Détection de noms propres absente** — `kern-anon` ne masque que des entités à motif
  fixe (IBAN, téléphone FR, email, NIR, SIREN/SIRET, carte bancaire...), aucun moteur
  NLP/ONNX n'est branché. Un nom en texte libre ("Jean Dupont") n'est pas masqué
  aujourd'hui — documenté explicitement dans le SKILL.md du skill, pas un oubli silencieux.

### Besoin #2 — Copilote de mémorandum : fait, vérifié en réel (2026-08-06)
- Enchaîné dans le MÊME graphe/run que le besoin #1 (choix utilisateur : kern-orch n'a pas
  de partage d'état entre runs, donc "un seul flux" veut dire un seul run).
  `extraction_validee → memo_prep → masquage_memo → redaction_memo → demasquage_memo →
  confirm_memo`.
- Les notes du premier entretien arrivent via `nudge` pendant la pause à
  `confirm_extraction` — pas de nouveau mécanisme, réutilise `mailbox.Nudge` existant.
  Erreur claire si absentes (pas d'historique client inventé).
- Même discipline de masquage PII que le besoin #1, clés d'état dédiées
  (`anonymizeMemoInput`/`deanonymizeMemoOutput`) pour ne jamais écraser l'état du besoin
  #1 — les deux (dossier structuré + draft de mémo) coexistent dans l'état final.
- **Bug réel trouvé en direct** : `claude -p` a répondu avec un JSON valide suivi de texte
  libre malgré l'instruction "réponds UNIQUEMENT avec un objet JSON" — `json.loads` sur
  toute la chaîne échouait ("Extra data"). Corrigé avec une extraction tolérante
  (`json.JSONDecoder().raw_decode` sur le premier `{`, ignore ce qui suit) — même leçon
  générique que le bug de détection de plateforme Telegram/X.

### Reste à faire — besoin #2
- Pas d'édition du draft de mémorandum dans une UI (comme le calendrier marketing de
  Kern-UI) — aujourd'hui uniquement consultable via l'état du run.

### ⚠️ À traduire avant de construire, pas à copier tel quel
Le document source décrit une architecture générique (Redis Vault, pgvector chiffré,
bus Pub/Sub, "Swarms") qui ne correspond pas à l'architecture Kern réelle et prouvée
(graphe `kern-orch`, nœuds `agent`/`tool`/`approval`, adaptateur Python `agent_cli.py`
par skill, garde-fous G2 vérifiés en conditions réelles). À traiter comme un **besoin
métier à satisfaire**, pas un plan technique à implémenter littéralement — le patron
`community-management-agency` (stratège → validation → rédaction → validation →
exécution) est le point de départ réaliste, pas un système multi-agents Pub/Sub à
construire de zéro.

### Besoin métier tel que décrit dans le document (résumé fidèle)

**1. Acquisition & Document Swarm** (Front-Office / OCR)
- Qualification initiale du souscripteur (faisabilité, taux d'endettement préliminaire,
  listes noires de solvabilité).
- OCR + vérification documentaire (extraction PDF, authenticité des avis d'imposition,
  réconciliation des relevés bancaires).

**2. Financial Engineering Swarm** (Risk & Restructuring)
- Modélisation de l'endettement actuel + ingénierie de la solution cible (simulation de
  prêt consolidé, calcul du reste à vivre).
- Scoring risque selon les grilles des banques partenaires (ratio d'endettement, saut de
  charge, stress test taux).

**3. Banking & Placement Swarm** (Négociation & Soumissions)
- Packaging et soumission automatisée du dossier aux banques mandataires, suivi des
  statuts d'approbation.
- Optimisation assurance emprunteur (délégation) et garantie (hypothèque/caution).

**4. Compliance & Operations Swarm** (Legal & Funding)
- Conformité réglementaire : devoir de conseil (DDA), FISE/FSI, contrôle LCB-FT, seuil de
  taux d'usure, listes PEP/sanctions.
- Orchestration des déblocages de fonds, remboursement des créanciers, clôture.

**Workflows critiques identifiés dans le document** :
- Lead → dossier pré-accepté : capture documentaire → ingénierie financière → scoring/
  matching bancaire.
- Soumission → négociation → conformité : package DDA → soumission multi-banques →
  optimisation assurance, sous contrainte du taux d'usure.

**Événements métier notés** (à retraduire en routage `kern-orch`, pas en bus Pub/Sub) :
documents vérifiés → déclenche le calcul d'endettement ; fraude détectée → gel du dossier
+ alerte humaine ; taux d'usure dépassé → bascule assurance ou rallongement de durée ;
offre émise → décompte de remboursement.

### Méthode de cadrage (2026-08-06)
Découpage global d'abord (quels sous-besoins, dans quel ordre), puis chaque besoin
détaillé un par un au moment de l'attaquer — pas tout spécifié à l'avance. L'utilisateur a
déjà des idées sur le découpage, à poser au moment de s'y mettre plutôt qu'anticipées ici.

### Découpage global acté (2026-08-06)
Recoupe le besoin générique du HTML avec le vrai besoin déjà qualifié par le prospect
AvelFinances (`prospect/prospect_courtier.md`, hors de ce dépôt) — le second prime sur le
premier quand ils divergent.

| # | Besoin | Swarm HTML correspondant | Statut |
|---|---|---|---|
| 1 | Extraction documentaire (OCR, reste à vivre, crédits en cours) | Acquisition & Document Swarm | à cadrer en premier — tout le reste en dépend |
| 2 | Copilote de mémorandum (draft d'instruction de dossier) | Financial Engineering Swarm (partiel) | réutilise le patron stratège → rédaction déjà prouvé |
| 3 | Relances Telegram (pièces manquantes) | *(absent du HTML)* | connecteur Telegram déjà construit — gain le plus rapide |
| 4 | RAG critères banques partenaires | Banking & Placement Swarm (partiel) | dépend de `kern-memory`, pas encore construit |
| — | Soumission bancaire automatisée + conformité réglementaire | Banking & Placement + Compliance Swarm | **hors scope court terme** — non demandé par le prospect, suppose des API banques absentes |

### Besoin #1 — Extraction documentaire : cadrage (2026-08-06)

**Le vrai enjeu n'est pas la précision OCR, c'est l'ordre des étapes.** L'argumentaire déjà
vendu au prospect (« vos données clients ne sortent jamais en clair ») impose que le
masquage PII (`kern-anon`, brique réelle du monorepo) arrive **avant** qu'un modèle de
raisonnement touche le texte. L'OCR doit donc être une extraction pure (texte brut), jamais
un modèle qui "comprend" le document — sinon la donnée part en clair vers un fournisseur
externe avant tout masquage possible.

**Pipeline retenu :**
```
Liasse (PDF/images)
  → découpage par page/pièce (évite les timeouts sur un gros PDF, échec isolé par page)
  → OCR pur (texte brut) — moteur switchable, voir ci-dessous
  → kern-anon masque le PII dans le texte extrait
  → agent Claude interprète le texte masqué : revenus, reste à vivre, crédits en cours, agios
  → résultat démasqué seulement pour la fiche finale (kern-anon fait le round-trip)
```

**Moteur OCR : deux moteurs, switchable par réglage (2026-08-06, acté)**
- **Local par défaut** (Tesseract ou PaddleOCR auto-hébergé) — fonctionne sans rien
  configurer, zéro donnée qui sort avant même le masquage, cohérent à 100% avec
  l'argumentaire vendu.
- **API cloud en option** (Azure Document Intelligence / Google Document AI / Mistral OCR,
  hébergement UE à privilégier pour le DPA) — activée dès qu'une clé API est renseignée
  dans les réglages, repli automatique sur le moteur local si absente. Même patron que
  `send_telegram`/`send_x` dans `community-management-agency` (vide = repli sûr, configuré
  = vrai appel) — pas une nouvelle idée d'architecture, une réutilisation directe.
- Reste à trancher au moment de coder : quelle(s) API(s) cloud précisément proposer
  (probablement Mistral OCR en premier — fournisseur EU, cohérent avec le reste), et le
  nom du réglage/variable d'environnement.

**Format de sortie de l'extraction (2026-08-06, acté)**

Le besoin dit "sans saisie manuelle dans un tableau Excel" — la sortie doit donc être de
la **donnée structurée**, pas du texte libre comme pour la comm. Chaque enregistrement
porte sa source (document + traçabilité) et un statut de confiance, jamais une valeur
affirmée sans preuve — même discipline anti-invention que partout ailleurs dans le projet.

```json
{
  "revenus": [
    {"source": "Salaire", "montant_mensuel": 2400, "document_source": "avis_imposition_2025.pdf", "statut": "confirmé"}
  ],
  "credits_en_cours": [
    {"etablissement": "Cetelem", "mensualite": 180, "capital_restant_du": null, "document_source": "releve_juin.pdf", "statut": "à vérifier"}
  ],
  "incidents": [
    {"type": "agios", "date": "2026-05-14", "montant": 45, "document_source": "releve_mai.pdf"}
  ],
  "reste_a_vivre": {"montant": null, "methode_calcul": "revenus_totaux - charges_totales - credits_totaux", "statut": "données incomplètes"},
  "pieces_manquantes": ["3e bulletin de salaire", "avis d'imposition N-1"]
}
```

`pieces_manquantes` alimente directement le besoin #3 (relances Telegram) plus tard —
même sortie, deux consommateurs.

**Découpage par page dans le graphe `kern-orch` (2026-08-06, acté)**

Pas de boucle native dans le moteur de graphe (topologie statique, niveaux synchrones) —
et ce n'est pas nécessaire. Même patron que `community-management-agency` : **un seul
nœud** ("extraction") fait le travail, le découpage par page/pièce est une boucle interne
à l'implémentation Python du nœud (comme `run_strategiste` fait déjà plusieurs choses en
interne), pas une structure visible dans le graphe. Le graphe reste simple : réception →
extraction (boucle interne : découpe → OCR par page → assemble) → masquage kern-anon →
interprétation → validation humaine si besoin.

**Moteur cloud retenu (2026-08-06, acté)** : Mistral OCR — fournisseur EU, cohérent avec le
reste. Repli local (Tesseract/PaddleOCR) si aucune clé Mistral configurée, même patron que
Telegram/X.

**Reste ouvert au moment de coder** : seuil de découpage par page, bibliothèque de
découpage PDF à utiliser côté Python.

**Ingestion des documents (2026-08-06, acté)**

Gap découvert en construisant le graphe : `POST /api/v1/dispatch` (kern-orch) ne porte
qu'un champ `text`, aucun mécanisme de fichier n'existe nulle part dans kern-orch ni
kern-ui. L'utilisateur veut à terme trois canaux : upload depuis l'UI, réception via
Telegram, dépôt dans un dossier/chemin fourni. Découpage retenu pour ne pas tout bloquer
sur le plus gros chantier :

| Canal | Statut | Effort |
|---|---|---|
| Chemin de fichier / dossier surveillé | **construit dans cette passe** | zéro nouvelle API, le node `extraction` lit un `document_path` dans le state |
| Réception Telegram (documents envoyés au bot) | reste à faire | connecteur Telegram déjà réel (`send_telegram`) mais réception de fichiers = nouveau code (`getUpdates`/webhook + téléchargement du fichier via l'API Telegram) |
| Upload réel depuis l'UI (route multipart) | reste à faire | chantier cross-repo à part entière (kern-orch + kern-ui), pas un sous-produit de ce besoin |

Le node `extraction` de `courtage-extraction` lit `state["document_path"]` (chemin
absolu local) pour cette première version — c'est le seul canal réellement câblé, les
deux autres sont des besoins d'ingestion séparés à cadrer quand on s'y attaque, pas des
détails d'implémentation de l'extraction elle-même.

---

## 3. Agence de restauration (`agence_restauration.html`)

**Statut** : pas commencé, pas encore cadré. Besoin source existant dans ce dossier
(`agence_restauration.html`) — même remarque que ci-dessus : à traduire vers l'architecture
Kern réelle avant de construire, pas à copier tel quel. Non détaillé ici tant que ce n'est
pas la priorité.
