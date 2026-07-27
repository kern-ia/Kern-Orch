---
id: 0012
feature: registry-contract
branch: feature/registry-contract
status: done
files:
  - internal/report/registry.go
  - internal/report/http.go
  - internal/topology/describe.go
  - internal/cmd/commands.go
  - internal/cmd/runtime.go
  - internal/config/config.go
  - contracts/kern.registry.v1.json
tests:
  - internal/report/registry_test.go
  - internal/cmd/publish_skills_test.go
  - internal/topology/describe_test.go
decisions:
  - "2026-07-27 : `kern.registry/v1` publié en push sur `KERN_REGISTRY_REPORT_URL` — variable distincte de `KERN_STEP_REPORT_URL`, jamais une route dérivée : l'URL est tout le contrat"
  - "2026-07-27 : `RegistryPublisher` est un type SÉPARÉ de `HTTPReporter` — deux endpoints, deux URL configurées, une responsabilité chacun"
  - "2026-07-27 : `postJSON` extrait et partagé par les deux — un timeout ou un message d'erreur ne doit pas dépendre du contrat qui voyage"
  - "2026-07-27 : `Publish` RETOURNE son erreur, contrairement au hook de step — il tourne hors du graphe, l'appelant décide ; c'est `cmd` qui garantit qu'un run ne meurt pas d'un sink cassé"
  - "2026-07-27 : `DeclaredNode.Skill` ajouté — la référence `skill:` du YAML était lue puis jetée ; sans elle un consommateur ne peut relier un run à un catalogue"
  - "2026-07-27 : publication au démarrage de `run` ET commande `publish-skills` — sinon un sink reste vide tant qu'aucun graphe n'a tourné"
---

**Quoi** : kern-orch publie son registre de skills. Le catalogue existait depuis
`0004-skills` mais ne sortait que sur le stdout de `list-skills` ; il traverse maintenant un
contrat. La topologie gagne au passage le `skill` que chaque nœud agent référence.

**Frontière** : kern-orch ne connaît aucun consommateur. Il poste sur une URL configurée et
ne sait rien de ce qu'on en fait — pas de nom de vue, pas de forme de route, pas de module
importé. Un second sink se branche sans toucher une ligne ici.

**Pièges** :
- Deux fichiers de test asserted la fixture v2 (`contract_test.go` ET `v2_test.go`) : patcher
  l'un laissait l'autre rouge. Le doublon est voulu (mirroir), il faut juste les toucher
  ensemble.
- `list-skills` prenait `--skills-dir` avec le défaut littéral `"skills"` au lieu de
  `config.FromEnv().SkillsDir` : `publish-skills` utilise le second, sinon `KERN_SKILLS_DIR`
  serait ignoré par une commande sur deux.
