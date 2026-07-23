# Brainstorming — recherches & décisions

Les états de l'art et notes de décision qui ont précédé (ou accompagné) les épics. Ce ne sont
pas des specs ni de la doc de référence — ce sont les **analyses ayant mené aux choix**. La
spec vit à la racine (`harnais-agentique-CDC-v2.md`), le planning dans `ROADMAP.md`, la
référence dans `ARCHITECTURE.md` / `GLOSSAIRE.md`.

| Fiche | Question posée | Décision / issue |
|---|---|---|
| [etat_de_lart.md](etat_de_lart.md) | Projet valable ? Répond-on au marché ? | Positionnement marché 2026 |
| [OBSERVABILITY.md](OBSERVABILITY.md) | LangSmith pour l'observabilité ? En Go ? | **OTLP/GenAI** comme socle ; LangSmith écarté (→ EPIC-11 / kern-obs) |
| [kern-memory-etat-de-lart.md](kern-memory-etat-de-lart.md) | Techno mature pour la mémoire agnostique ? | **kern-memory 100% Go**, chromem-go par défaut (→ EPIC-13) |
