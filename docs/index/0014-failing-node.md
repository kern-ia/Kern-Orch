---
id: 0014
feature: failing-node
branch: feature/failing-node
status: done
files:
  - internal/graph/engine.go
  - internal/report/http.go
  - internal/cmd/runtime.go
  - internal/cmd/commands.go
  - contracts/kern.step-event.v2.failure.json
tests:
  - internal/graph/engine_test.go
  - internal/report/contract_test.go
decisions:
  - "2026-07-28 : `LevelError` typée, `errors.As` côté appelant — l'id du nœud existait dans `outcome` et se noyait dans `fmt.Errorf`"
  - "2026-07-28 : `Error()` renvoie le message d'avant à l'identique — le message est un contrat avec les humains et avec les tests existants"
  - "2026-07-28 : toutes les erreurs du niveau collectées, plus seulement la première — `wg.Wait()` les avait déjà"
  - "2026-07-28 : `Nodes` trié, et champ omis quand vide — un sink distingue « aucun nommé » de « aucun échec »"
---

**Quoi** : le moteur nomme les nœuds qui ont échoué, et l'information traverse enfin le
contrat jusqu'au consommateur.

**Piège** : `ReportFailure` a gagné un paramètre ; trois fichiers de test l'appelaient.
Le compilateur les a tous attrapés — signature élargie plutôt que champ optionnel.
