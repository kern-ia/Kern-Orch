---
id: okf-0005
feature: skills-registry
branch: feature/skills-registry
status: done
files:
  - internal/skills/skills.go
  - internal/cmd/commands.go
tests:
  - internal/skills/skills_test.go
decisions:
  - "2026-07-20 : type tool|agent lu dans le frontmatter YAML de SKILL.md (§6.5) ; name par défaut = nom du dossier"
  - "2026-07-20 : frontmatter parsé maison (entre les deux ---) puis yaml.Unmarshal ; type manquant/invalide = erreur"
  - "2026-07-20 : dossier skills absent = registry vide (skills optionnels) ; list-skills via --skills-dir"
---

**Quoi** : Registry des skills. `skills.Load(dir)` scanne les sous-dossiers contenant un
SKILL.md, parse le frontmatter (`name`, `type: tool|agent`, `description`). `list-skills`
affiche la table triée par nom. Un sous-dossier sans SKILL.md est ignoré.

**Pièges** : le type est obligatoire et validé (tool|agent) — un skill sans type fait échouer
le Load, volontairement (pas de défaut silencieux).
