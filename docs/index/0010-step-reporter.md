---
id: okf-0010
feature: step-reporter
branch: feature/step-reporter
status: done
files:
  - internal/report/http.go
  - internal/config/config.go
  - internal/cmd/runtime.go
  - internal/cmd/commands.go
tests:
  - internal/report/http_test.go
  - internal/cmd/runtime_hooks_test.go
decisions:
  - "2026-07-26 : reporter dans internal/report, pas dans cmd — l'infra dépend de graph, jamais l'inverse (même direction que agentrunner et checkpoint)"
  - "2026-07-26 : `KERN_STEP_REPORT_URL` porte l'URL complète du sink ; kern-orch ignore la forme de ses routes, la brique reste agnostique"
  - "2026-07-26 : le hook retourne TOUJOURS nil — un sink lent, cassé ou absent ne doit jamais faire échouer un run ; l'erreur part sur stderr"
  - "2026-07-26 : `multiStep` compose les hooks pour le slot unique de `Engine.OnStep` ; checkpoint d'abord (durabilité), reporter ensuite (best-effort). Le package graph n'est pas modifié"
  - "2026-07-26 : le payload transporte le state APLATI via Keys()/Get(), pas graph.State — sérialiser le State exporterait l'enveloppe interne (zones, frozen, step) dans le contrat"
  - "2026-07-26 : hook nil quand aucune URL n'est configurée, pour que l'appelant l'omette de la chaîne sans brancher"
---

**Quoi** : kern-orch peut pousser chaque niveau de graphe terminé vers un sink HTTP.
`KERN_STEP_REPORT_URL` non défini = comportement inchangé, aucune requête. `run` et `resume`
composent le hook de checkpoint et le reporter via `multiStep`. Premier consommateur :
kern-ui, qui affiche les runs en direct.

**Pièges** :
- `Engine.OnStep` n'a qu'un seul slot (`e.onStep = f`) : enregistrer un second hook écrase
  le premier. D'où `multiStep`, côté appelant, sans toucher à `graph`.
- Le hook fire par niveau, pas par nœud : la granularité observable est la frontière.
- `graph.StepInfo.Frontier` est nil en fin de run ; le contrat exige une liste vide, sinon
  le sink ne peut pas distinguer « pas de frontière » de « champ absent ».

**E2E** : `hello`, `freeze`, `parent` et `child` exécutés au binaire contre un kern-ui réel,
runs affichés en direct dans le navigateur. Vérifié aussi avec le sink éteint puis avec une
URL invalide : erreurs sur stderr, runs terminés, code de sortie 0.

## Contrat exécutable (2026-07-26)

`contracts/kern.step-event.v1.json` — même fichier dans les deux repos.
`internal/report/contract_test.go` capture ce que le vrai Hook met sur le fil et le compare
à la fixture ; côté kern-ui, `internal/httpapi/contract_test.go` la fait passer par la vraie
route d'ingestion. Renommer un champ d'un côté fait rougir les deux suites — vérifié en
cassant délibérément `graph` → `graph_name`.

Décision : **la dérive de contrat est attrapée par des tests, pas par de la discipline.** Une
commande `diff` à lancer à la main reposait sur le fait de s'en souvenir, ce qui est
précisément la panne qu'elle prétendait éviter.
